// Файл предоставляет кастомный хэндлер для slog, который выводит логи в цвете
// с помощью github.com/fatih/color.
package logging

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/fatih/color"
	"golang.org/x/exp/slog"
)

// LevelFatal — уровень фатальной ошибки (выше Error).
const LevelFatal = slog.Level(12)

// Цвета уровней логирования.
var levelColors = map[slog.Level]*color.Color{
	LevelFatal:      color.New(color.FgRed, color.Bold),
	slog.LevelError: color.New(color.FgRed),
	slog.LevelWarn:  color.New(color.FgYellow),
	slog.LevelInfo:  color.New(color.FgGreen),
	slog.LevelDebug: color.New(color.FgCyan),
}

// levelNames — короткие имена уровней.
var levelNames = map[slog.Level]string{
	LevelFatal:      "FATAL",
	slog.LevelError: "ERRO",
	slog.LevelWarn:  "WARN",
	slog.LevelInfo:  "INFO",
	slog.LevelDebug: "DEBU",
}

// ColorHandler — кастомный хэндлер slog с цветным выводом.
type ColorHandler struct {
	mu     *sync.Mutex
	out    io.Writer
	level  slog.Level
	attrs  []slog.Attr
	groups []string
}

// NewColorHandler создаёт новый хэндлер, пишущий в out записи не ниже level.
func NewColorHandler(out io.Writer, level slog.Level) *ColorHandler {
	return &ColorHandler{mu: &sync.Mutex{}, out: out, level: level}
}

// Enabled сообщает, обрабатываются ли записи указанного уровня.
func (h *ColorHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level
}

// WithAttrs возвращает копию хэндлера с добавленными атрибутами.
func (h *ColorHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	na := make([]slog.Attr, 0, len(h.attrs)+len(attrs))
	na = append(na, h.attrs...)
	na = append(na, attrs...)
	return &ColorHandler{mu: h.mu, out: h.out, level: h.level, attrs: na, groups: h.groups}
}

// WithGroup возвращает копию хэндлера с добавленной группой.
func (h *ColorHandler) WithGroup(name string) slog.Handler {
	ng := make([]string, 0, len(h.groups)+1)
	ng = append(ng, h.groups...)
	ng = append(ng, name)
	return &ColorHandler{mu: h.mu, out: h.out, level: h.level, attrs: h.attrs, groups: ng}
}

// Handle выводит одну запись лога в цвете.
func (h *ColorHandler) Handle(_ context.Context, r slog.Record) error {
	// Информация об источнике вызова: последняя директория, файл, строка.
	frame, _ := runtime.CallersFrames([]uintptr{r.PC}).Next()
	dir := filepath.Base(filepath.Dir(frame.File))
	file := filepath.Base(frame.File)
	line := frame.Line

	// Цвет и имя уровня.
	lc, ok := levelColors[r.Level]
	if !ok {
		lc = color.New(color.FgWhite)
	}
	name, ok := levelNames[r.Level]
	if !ok {
		name = r.Level.String()
	}

	// Дополнительные атрибуты: названия лейблов — тёмным цветом, как дата/время.
	keyColor := color.New(color.FgHiBlack)
	srcColor := color.New(color.FgHiMagenta)
	var attrs strings.Builder
	r.Attrs(func(a slog.Attr) {
		attrs.WriteString(" ")
		attrs.WriteString(keyColor.Sprint(a.Key + "="))
		// Значение с пробелами оборачиваем в кавычки, иначе выводим как есть.
		val := a.Value.String()
		if strings.ContainsAny(val, " \t") {
			attrs.WriteString(keyColor.Sprint(`"`))
			attrs.WriteString(val)
			attrs.WriteString(keyColor.Sprint(`"`))
		} else {
			attrs.WriteString(val)
		}
	})
	// Секция source в конце, как обычный атрибут (без пробелов).
	fmt.Fprintf(&attrs, " %s%s/%s:%d", srcColor.Sprint("source="), dir, file, line)

	h.mu.Lock()
	defer h.mu.Unlock()

	// Время, уровень и описание (сообщение) разными цветами.
	fmt.Fprintf(h.out, "%s %s %s%s\n",
		color.New(color.FgHiBlack).Sprint(r.Time.Format("2006-01-02 15:04:05")),
		lc.Sprint(name),
		color.New(color.FgWhite).Sprint(r.Message),
		attrs.String(),
	)
	return nil
}

// NewColorLogging создаёт логгер с цветным выводом в консоль
// (только для интерактивного режима, см. cmd/root.go).
func NewColorLogging(level string) *slog.Logger {
	l := slog.LevelInfo
	if v, ok := logLevelMap[level]; ok {
		l = v
	}

	baseHandler := NewColorHandler(os.Stdout, l)

	log := slog.New(baseHandler)

	return log
}
