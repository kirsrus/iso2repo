//go:build windows

package sevenz

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/cockroachdb/errors"
)

// find7z ищет запускной файл 7z в системе Windows. Возвращается абсолютный
// путь к файлу.
func (m *SevenZ) find7z() (string, bool) {
	const bin = "7z.exe"

	// 1. Поиск в PATH (exec.LookPath использует кеш и корректно обрабатывает расширения)
	if p, err := exec.LookPath(bin); err == nil {
		return p, true
	}

	// 2. Стандартные пути установки
	dirs := []string{
		os.Getenv("ProgramFiles"),
		os.Getenv("ProgramFiles(x86)"),
	}
	for _, dir := range dirs {
		if dir == "" {
			continue
		}
		p := filepath.Join(dir, "7-Zip", bin)
		if _, err := os.Stat(p); err == nil {
			return p, true
		}
	}

	return "", false
}

// exec7zOnce запускает программу 7z и возвращает её вывод в виде одной строки.
func (m *SevenZ) exec7zOnce(args []string) (string, error) {
	m.log.Debug(fmt.Sprintf("7z: exec - %s %s", m.sevenZPath, strings.Join(args, " ")))

	cmd := exec.Command(m.sevenZPath, args...)
	out, err := cmd.CombinedOutput()
	output := strings.TrimSpace(string(out))

	if err != nil {
		m.log.Debug(fmt.Sprintf("7z: exec - размер буфера %d", len(output)))
		if len(output) > 0 {
			return output, nil
		}
		// Извлекаем код выхода напрямую из ExitError без regexp
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return "", errors.Errorf("код выхода %d", exitErr.ExitCode())
		}
		return "", errors.WithStack(err)
	}

	return output, nil
}
