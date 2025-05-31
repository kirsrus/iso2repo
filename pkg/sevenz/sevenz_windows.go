package sevenz

import (
	"bytes"
	"fmt"
	"github.com/juju/errors"
	"io"
	"os/exec"
	"regexp"
	"strings"
)

// find7ZBin ищет запускной файл 7z
func (m SevenZ) find7ZBin() (string, error) {
	return Find7Z()
}

// Запускает программу 7z и возвращает его вывода в виде одной строки.
func (m SevenZ) exec7zOnce(args []string) (string, error) {
	m.log.Debugf("exec - %s %s", m.sevenZPath, strings.Join(args, " "))
	subProcess := exec.Command(m.sevenZPath, args...)
	buffOut := new(bytes.Buffer)

	subProcess.Stdout = buffOut
	subProcess.Stderr = buffOut

	if err := subProcess.Start(); err != nil {
		return "", errors.Trace(err)
	}

	if err := subProcess.Wait(); err != nil {
		buffOutStr := strings.TrimSpace(buffOut.String())
		m.log.Debugf("exec - размер буфера %d", len(buffOutStr))
		if len(buffOutStr) != 0 {
			return buffOutStr, nil
		}

		match := regexp.MustCompile(`exit status (-?\d+)`).FindStringSubmatch(err.Error())
		if len(match) == 0 {
			return "", errors.Annotate(errors.New("код выхода -1"), fmt.Sprintf("%s %s", m.sevenZPath, strings.Join(args, " ")))
		} else {
			return "", errors.Annotate(errors.Errorf("код выхода %s", match[1]), fmt.Sprintf("%s %s", m.sevenZPath, strings.Join(args, " ")))
		}
	}

	return strings.TrimSpace(buffOut.String()), nil
}

// ReadFile читает выбранный файл file в ридер out
func (m *SevenZ) ReadFile(fileAbsPath string, out io.Writer) error {
	file := strings.TrimLeft(fileAbsPath, "/")
	file = m.pathPrefix + file

	args := []string{"e", m.ISOPath, "-so", file}

	m.log.Debugf("exec - %s %s", m.sevenZPath, strings.Join(args, " "))
	subProcess := exec.Command(m.sevenZPath, args...)

	subProcess.Stdout = out

	if err := subProcess.Start(); err != nil {
		return errors.Trace(err)
	}

	if err := subProcess.Wait(); err != nil {
		match := regexp.MustCompile(`exit status (-?\d+)`).FindStringSubmatch(err.Error())
		if len(match) == 0 {
			return errors.Annotate(errors.New("код выхода -1"), fmt.Sprintf("%s %s", m.sevenZPath, strings.Join(args, " ")))
		} else {
			return errors.Annotate(errors.Errorf("код выхода %s", match[1]), fmt.Sprintf("%s %s", m.sevenZPath, strings.Join(args, " ")))
		}
	}

	return nil
}
