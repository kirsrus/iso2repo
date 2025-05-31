package logging

import (
	"fmt"
	"path"
	"runtime"
	"strings"

	"github.com/sirupsen/logrus"
)

// LogrusContextHook хак к logrus
type LogrusContextHook struct{}

// Levels возвращает текущие уровни
func (hook LogrusContextHook) Levels() []logrus.Level {
	return logrus.AllLevels
}

// Fire выбирает информацию из текущего места вызова лога, чтобы вернуть имя файла, функцию и номер
// строки вызова
func (hook LogrusContextHook) Fire(entry *logrus.Entry) error {
mainloop:
	for i := 0; ; i++ {
		if pc, file, line, ok := runtime.Caller(i); ok {
			funcName := path.Base(runtime.FuncForPC(pc).Name()) // Имя функции

			// Пропускаем вызов данной структуры
			if strings.Contains(funcName, ".LogrusContextHook.") {
				continue
			}

			// Пропускаем все служебные модули
			for _, v := range []string{"LogrusContextHook.", "logrus.", "runtime.", "testing."} {
				if strings.HasPrefix(funcName, v) {
					continue mainloop
				}
			}

			// Если дошли сюда, значит дошли до точки логирования
			entry.Data["file"] = fmt.Sprintf("%s:%d", path.Base(file), line)
			break
		} else {
			break
		}
	}

	return nil
}
