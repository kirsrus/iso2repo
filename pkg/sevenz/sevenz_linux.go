//go:build linux

package sevenz

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/cockroachdb/errors"
)

// find7z ищет запускной файл 7z в системе Linux.
func (m *SevenZ) find7z() (string, bool) {
	const bin = "7z"

	// Ищем по всем путям в PATH.
	if p, err := exec.LookPath(bin); err == nil {
		return p, true
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
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return "", errors.Errorf("код выхода %d", exitErr.ExitCode())
		}
		return "", errors.WithStack(err)
	}

	return output, nil
}
