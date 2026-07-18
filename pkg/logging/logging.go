package logging

import (
	"log/slog"
	"os"

	"github.com/kirsrus/iso2repo/pkg/logging/tint"
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

// NewLoggingStringLevel создаёт лог с уровнем в текстовом виде.
func NewLoggingWithStringLevel(level string, deph int) *slog.Logger {
	l, ok := logLevelMap[level]
	if !ok {
		l = slog.LevelInfo
	}

	return NewLogging(l, deph)
}

// NewLogging создает лог с уровнем и глубиной вывода истоника возникновения лога.
func NewLogging(level slog.Level, deph int) *slog.Logger {

	// Custom level names for alignment (4 characters each).
	levelNames := map[slog.Level]string{
		slog.LevelError: "erro",
		slog.LevelWarn:  "warn",
		slog.LevelInfo:  "info",
		slog.LevelDebug: "debu",
	}

	// Create a text handler that writes to stderr.
	// ReplaceAttr replaces the built-in slog.Level value with our custom string.
	inner := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level:     level,
		AddSource: false,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if a.Key == slog.LevelKey {
				if l, ok := a.Value.Any().(slog.Level); ok {
					if name, ok := levelNames[l]; ok {
						return slog.String("level", name)
					}
				}
			}
			return a
		},
	})

	// Wrap it with our custom handler.
	log := slog.New(NewHandler(inner, WithDepth(deph)))

	return log
}

// NewTintLogging создаёт логгер для отображения в удобном виде в косоли.
func NewTintLogging(level string) *slog.Logger {
	l, ok := logLevelMap[level]
	if !ok {
		l = slog.LevelInfo
	}

	opts := tint.Options{
		Level:      l,
		AddSource:  true,
		TimeFormat: "2006.01.02T15:04:05.000MST", // 2023.11.22T01:06:52.121+05
	}

	log := slog.New(tint.NewTextHandler(os.Stdout, &opts))

	return log
}
