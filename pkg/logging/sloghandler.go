// Package logging provides a slog.Handler that extracts file:line information
// from errors created with github.com/cockroachdb/errors and includes it in log output.
package logging

import (
	"context"
	"fmt"
	"log/slog"
	"runtime"
	"strings"

	"github.com/cockroachdb/errors"
)

// Ensure Handler implements slog.Handler.
var _ slog.Handler = (*Handler)(nil)

// Handler is a slog.Handler wrapper that enriches error records with
// source location (file:line) extracted from cockroachdb/errors stack traces.
type Handler struct {
	inner slog.Handler
	depth int // number of path components to show (0 = full relative path)
}

// NewHandler creates a new Handler that delegates to the provided inner handler.
// Options can be passed to configure the handler, e.g. WithDepth(2).
func NewHandler(inner slog.Handler, opts ...Option) *Handler {
	h := &Handler{
		inner: inner,
		depth: 0, // default: full relative path from module root
	}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

// Option configures a Handler.
type Option func(*Handler)

// WithDepth sets the number of trailing path components to show in source/origin.
// Example: WithDepth(2) on "level1/level2/level3/file.go:42" produces "level3/file.go:42".
func WithDepth(n int) Option {
	return func(h *Handler) {
		if n < 0 {
			n = 0
		}
		h.depth = n
	}
}

// Enabled reports whether the handler is enabled for the given level.
func (m *Handler) Enabled(ctx context.Context, level slog.Level) bool {
	return m.inner.Enabled(ctx, level)
}

// Handle processes a log Record. If the record contains an error attribute
// that carries a cockroachdb/errors stack trace, it extracts the originating
// file:line, removes the verbose error attribute, and adds compact "source"
// (call site of slog.Error) and "origin" (where the error was created) instead.
func (m *Handler) Handle(ctx context.Context, record slog.Record) error {
	// Collect all attributes from the record.
	var attrs []slog.Attr
	record.Attrs(func(a slog.Attr) bool {
		attrs = append(attrs, a)
		return true
	})

	// Build a new record without the verbose error attribute.
	// We pass 0 for PC because we'll add our own source attribute manually.
	newRecord := slog.NewRecord(record.Time, record.Level, record.Message, 0)

	// Add source from the original record's PC (call site of slog.Error).
	if record.PC != 0 {
		if src := m.sourceFromPC(record.PC); src != "" {
			newRecord.AddAttrs(slog.String("source", src))
		}
	}

	for _, a := range attrs {
		// Check if this attribute is an error with a cockroachdb stack trace.
		if a.Value.Kind() == slog.KindAny {
			if err, ok := a.Value.Any().(error); ok && err != nil {
				if origin, msgs := m.extractChain(err); origin != "" {
					// Skip the verbose error attribute and add origin instead.
					newRecord.AddAttrs(slog.String("origin_error", msgs))
					newRecord.AddAttrs(slog.String("origin_source", origin))
					continue
				}
			}
		}
		// Keep all other attributes as-is.
		newRecord.AddAttrs(a)
	}

	return m.inner.Handle(ctx, newRecord)
}

// sourceFromPC extracts a short file:line from a program counter.
func (m *Handler) sourceFromPC(pc uintptr) string {
	frames := runtime.CallersFrames([]uintptr{pc})
	frame, _ := frames.Next()
	if frame.File == "" {
		return ""
	}
	return fmt.Sprintf("%s:%d", m.shortenPath(frame.File), frame.Line)
}

// extractChain walks the error chain to find the origin source location.
// Returns the origin source and the full error message (which already
// contains the chain joined by ": " via cockroachdb/errors formatting).
// Returns empty source if no stack trace is found anywhere in the chain.
func (m *Handler) extractChain(err error) (source string, messages string) {
	// Walk the error chain from outermost to innermost.
	// Find the deepest error that has a stack trace for origin_source.
	var deepestSource string

	for e := err; e != nil; e = errors.UnwrapOnce(e) {
		if src := m.extractSource(e); src != "" {
			deepestSource = src
		}
	}

	if deepestSource == "" {
		return "", ""
	}

	// Use the outermost error's Error() which already contains the full
	// chain joined by ": " (e.g. "добавленное сообщение: моя ошибка").
	return deepestSource, err.Error()
}

// extractSource extracts the originating file:line from a cockroachdb error.
// Returns empty string if no stack trace is found.
func (m *Handler) extractSource(err error) string {
	st := errors.GetReportableStackTrace(err)
	if st == nil || len(st.Frames) == 0 {
		return ""
	}

	// Frames are ordered oldest-first (Sentry convention).
	// The last user frame is the actual error origin (where errors.New was called).
	var originFile string
	var originLine int
	for i := len(st.Frames) - 1; i >= 0; i-- {
		f := st.Frames[i]
		if isUserFrame(f.Filename) {
			originFile = f.Filename
			originLine = f.Lineno
			break
		}
	}

	// Fallback: if no user frame found, use the last frame (most recent call).
	if originFile == "" && len(st.Frames) > 0 {
		last := st.Frames[len(st.Frames)-1]
		originFile = last.Filename
		originLine = last.Lineno
	}

	if originFile == "" {
		return ""
	}

	return fmt.Sprintf("%s:%d", m.shortenPath(originFile), originLine)
}

// WithAttrs returns a new Handler with the given attributes pre-attached.
func (m *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &Handler{inner: m.inner.WithAttrs(attrs), depth: m.depth}
}

// WithGroup returns a new Handler with the given group name.
func (m *Handler) WithGroup(name string) slog.Handler {
	return &Handler{inner: m.inner.WithGroup(name), depth: m.depth}
}

// isUserFrame returns true if the filename is likely a user's source file
// (not standard library, not cockroachdb, not runtime).
func isUserFrame(filename string) bool {
	if filename == "" {
		return false
	}
	// Skip standard library paths (no directory separator means std lib).
	if !strings.Contains(filename, "/") {
		return false
	}
	// Skip cockroachdb internal paths.
	if strings.Contains(filename, "cockroachdb") {
		return false
	}
	// Skip runtime paths.
	if strings.HasPrefix(filename, "runtime/") {
		return false
	}
	return true
}

// shortenPath shortens a file path according to the configured depth.
// depth=0: only filename (e.g. "file.go")
// depth=1: last 1 directory + filename (e.g. "level3/file.go")
// depth=2: last 2 directories + filename (e.g. "level2/level3/file.go")
// If the path has fewer components than depth, returns the full relative path.
func (m *Handler) shortenPath(path string) string {
	// First, try to make the path relative to the module root.
	rel := path
	if _, after, ok := strings.Cut(path, "error-log-concept"); ok {
		rel = strings.TrimLeft(after, "/\\")
	}

	// Normalize separators.
	rel = strings.ReplaceAll(rel, "\\", "/")

	// Split into components.
	parts := strings.Split(rel, "/")

	// depth <= 0: only the filename (last component).
	if m.depth <= 0 {
		return parts[len(parts)-1]
	}

	// If the path has fewer components than requested depth,
	// return the full relative path.
	if len(parts) <= m.depth {
		return rel
	}

	// Take only the last `depth` directory components + the filename.
	return strings.Join(parts[len(parts)-m.depth-1:], "/")
}
