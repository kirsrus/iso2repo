package logging

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"golang.org/x/exp/slog"
)

var (
	logLevelMap = map[string]slog.Level{
		"debug":   slog.LevelDebug,
		"info":    slog.LevelInfo,
		"warn":    slog.LevelWarn,
		"warning": slog.LevelWarn,
		"error":   slog.LevelError,
	}
)

// LevelFilterHandler оборачивает любой обработчик и пропускает только записи
// с уровнем не ниже заданного.
type LevelFilterHandler struct {
	inner    slog.Handler
	minLevel slog.Level
}

// Enabled определяет, должна ли обрабатываться запись с данным уровнем.
func (h *LevelFilterHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return level >= h.minLevel
}

// Handle передаёт запись внутреннему обработчику, если Enabled вернула true.
func (h *LevelFilterHandler) Handle(ctx context.Context, r slog.Record) error {
	return h.inner.Handle(ctx, r)
}

// WithAttrs возвращает новый обработчик с добавленными атрибутами.
func (h *LevelFilterHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &LevelFilterHandler{
		inner:    h.inner.WithAttrs(attrs),
		minLevel: h.minLevel,
	}
}

// WithGroup возвращает новый обработчик с добавленной группой.
func (h *LevelFilterHandler) WithGroup(name string) slog.Handler {
	return &LevelFilterHandler{
		inner:    h.inner.WithGroup(name),
		minLevel: h.minLevel,
	}
}

// CallerHandler оборачивает обработчик и добавляет в конец записи атрибут
// caller="файл:строка" с местом вызова лога.
type CallerHandler struct {
	inner slog.Handler
}

// Enabled делегирует проверку уровня внутреннему обработчику.
func (h *CallerHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

// Handle добавляет атрибут с местом вызова и передаёт запись внутреннему обработчику.
func (h *CallerHandler) Handle(ctx context.Context, r slog.Record) error {
	if r.PC != 0 {
		frame, _ := runtime.CallersFrames([]uintptr{r.PC}).Next()
		if frame.File != "" {
			dir := filepath.Base(filepath.Dir(frame.File))
			file := filepath.Base(frame.File)
			r.AddAttrs(slog.String("source", fmt.Sprintf("%s/%s:%d", dir, file, frame.Line)))
		}
	}
	return h.inner.Handle(ctx, r)
}

// WithAttrs возвращает новый обработчик с добавленными атрибутами.
func (h *CallerHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &CallerHandler{
		inner: h.inner.WithAttrs(attrs),
	}
}

// WithGroup возвращает новый обработчик с добавленной группой.
func (h *CallerHandler) WithGroup(name string) slog.Handler {
	return &CallerHandler{
		inner: h.inner.WithGroup(name),
	}
}

// NewTintLogging создаёт логгер для отображения в удобном виде в косоли.
func NewTintLogging(level string) *slog.Logger {
	l := slog.LevelInfo
	if v, ok := logLevelMap[level]; ok {
		l = v
	}

	baseHandler := slog.NewTextHandler(os.Stdout)

	callerHandler := &CallerHandler{
		inner: baseHandler,
	}

	filterHandler := &LevelFilterHandler{
		inner:    callerHandler,
		minLevel: l,
	}

	log := slog.New(filterHandler)

	return log
}
