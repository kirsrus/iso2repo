// Package sevenz предоставляет интерфейс для работы с утилитой 7z.
// Поддерживает поиск бинарника 7z в системе, выполнение команд,
// парсинг вывода и потоковое извлечение файлов из ISO-образов.
package sevenz

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/kirsrus/iso2repo/models"
	"github.com/spf13/cast"
)

var once sync.Once

// SevenZ предоставляет методы для работы с утилитой 7z.
// Содержит платформозависимую логику поиска бинарника и выполнения команд.
type SevenZ struct {
	log           *slog.Logger
	sevenZPath    string
	sevenZVersion string
}

// NewSevenZ ищет утилиту 7z в системе, определяет её версию и возвращает
// готовый к работе экземпляр SevenZ. Если 7z не найдена, возвращается ошибка.
func NewSevenZ(log *slog.Logger) (*SevenZ, error) {
	if log == nil {
		log = slog.New(slog.NewTextHandler(io.Discard, nil))
	}

	m := &SevenZ{
		log: log.With("sub", "7z"),
	}

	if !m.check7z() {
		return nil, errors.New("на комьютере не обнаружена утилита 7z, без неё невозможно работать с .iso файлами")
	}

	var err error
	m.sevenZVersion, err = m.read7ZVersion()
	if err != nil {
		return nil, err
	} else if m.sevenZVersion == "0.0.0" {
		m.log.Warn("не удалось определить номер версии 7z (результат работы программы не гарантирован)")
	} else {
		versionStr := strings.Split(m.sevenZVersion, ".")
		version := make([]int, 0)

		for _, v := range versionStr {
			version = append(version, cast.ToInt(v))
		}

		// Выводим отчёт или предупреждение только один раз.
		once.Do(func() {
			if version[0] < 16 || version[0] > 26 {
				m.log.Warn(fmt.Sprintf("версия 7z %s не проверялась (результат работы программы не гарантирован)", m.sevenZVersion))
			} else {
				m.log.Info(fmt.Sprintf("версия 7z %s", m.sevenZVersion))
			}
		})

	}

	return m, nil
}

// Path возвращает путь к бинарнику 7z.
func (m *SevenZ) Path() string {
	return m.sevenZPath
}

// Version возвращает версию 7z в формате "major.minor.0".
func (m *SevenZ) Version() string {
	return m.sevenZVersion
}

// ExecOnce запускает программу 7z с указанными аргументами и возвращает
// её вывод в виде одной строки. Платформозависимая реализация находится
// в sevenz_windows.go / sevenz_linux.go.
func (m *SevenZ) ExecOnce(args []string) (string, error) {
	return m.exec7zOnce(args)
}

// Open открывает файл внутри ISO для потокового чтения.
// При отмене ctx процесс 7z принудительно завершается.
func (m *SevenZ) Open(ctx context.Context, isoPath, filePath string) (io.ReadCloser, error) {
	file := strings.TrimLeft(filePath, "/")

	args := []string{"e", isoPath, "-so", file}
	m.log.Debug(fmt.Sprintf("7z: exec - %s %s", m.sevenZPath, strings.Join(args, " ")))

	// CommandContext автоматически убивает процесс при отмене ctx
	cmd := exec.CommandContext(ctx, m.sevenZPath, args...)

	pipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}

	if err := cmd.Start(); err != nil {
		pipe.Close()
		return nil, err
	}

	return &cmdReadCloser{pipe: pipe, cmd: cmd}, nil
}

// ListFiles получает список всех файлов в ISO образе и парсит их в
// древовидную структуру []models.Entry.
func (m *SevenZ) ListFiles(isoPath string) ([]models.Entry, error) {
	output, err := m.exec7zOnce([]string{"l", isoPath})
	if err != nil {
		return nil, err
	}

	result := make([]models.Entry, 0)

	reLine := regexp.MustCompile(`^(\d+-\d+-\d+)\s+(\d+:\d+:\d+)\s+(.|D)\.\.\.\.\s+(\d*)\s+(\d*)\s+([^ ]+)$`)

	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)

		match := reLine.FindStringSubmatch(line)
		if len(match) != 0 {
			newFileOrDir := models.Entry{
				IsDir: match[3] == "D",
				Size:  cast.ToInt64(match[4]),
				Name:  filepath.Base(match[6]),
			}

			// Нормализуем время
			testTime := match[1] + " " + match[2]
			t, err := time.Parse("2006-01-02 15:04:05", testTime)
			if err != nil {
				m.log.Warn(fmt.Sprintf("не удалось распарсить время создания файла/директории '%s'", testTime))
			}
			newFileOrDir.CreateAt = t

			// Нормализуем путь и интегрируем в дерево путей
			filePathParts := make([]models.Entry, 0)
			filePath := strings.ReplaceAll(match[6], "\\", "/")
			filePath = strings.TrimPrefix(filePath, "/")
			filePath = strings.TrimSuffix(filePath, "/")

			filePathSplit := strings.Split(filePath, "/")
			if len(filePathSplit) > 1 {
				for _, pathPart := range filePathSplit[:len(filePathSplit)-1] {
					filePathParts = append(filePathParts, models.Entry{
						IsDir:    true,
						Children: make([]models.Entry, 0),
						Name:     pathPart,
					})
				}
			}
			filePathParts = append(filePathParts, newFileOrDir)

			recurseFileAdd(filePathParts, 0, &result)
		}
	}

	return result, nil
}

// check7z проверяет наличие утилиты 7z. Если утилита обнаружена, возвращается true
// и путь к бинарнику сохраняется во внутренней переменной.
func (m *SevenZ) check7z() bool {
	path, ok := m.find7z()
	if !ok {
		return false
	}

	m.sevenZPath = path

	return true
}

// read7ZVersion определяет версию установленного 7z. Если версию определить
// не удалось, возвращается версия "0.0.0".
func (m *SevenZ) read7ZVersion() (string, error) {
	output, err := m.exec7zOnce([]string{})
	if err != nil {
		return "", err
	}

	// Регулярки для поиска версии в выводе 7z
	reList := []*regexp.Regexp{
		regexp.MustCompile(`^[0-9a-zA-Z-]+\s+(\d+\.\d+)`),
		regexp.MustCompile(`^[0-9a-zA-Z-]+\s+\[\d+]\s+(\d+\.\d+)`),
	}

	// Берём только первую строку — остальные не нужны
	firstLine := strings.TrimSpace(output)
	if idx := strings.IndexByte(firstLine, '\n'); idx >= 0 {
		firstLine = strings.TrimSpace(firstLine[:idx])
	}

	var match []string
	for _, re := range reList {
		if match = re.FindStringSubmatch(firstLine); len(match) > 0 {
			break
		}
	}
	if len(match) == 0 {
		return "0.0.0", nil
	}

	// Нормализация версии: гарантируем формат major.minor.0
	parts := strings.SplitN(match[1], ".", 2)
	major := cast.ToString(cast.ToInt(parts[0]))
	minor := "0"
	if len(parts) > 1 {
		minor = cast.ToString(cast.ToInt(parts[1]))
	}

	return major + "." + minor + ".0", nil
}

// recurseFileAdd рекурсивно добавляет элемент в дерево, создавая промежуточные
// директории при необходимости.
func recurseFileAdd(pathParts []models.Entry, positionInPath int, pathTree *[]models.Entry) {
	if positionInPath < len(pathParts) { // Добавляется директория
		if pathParts[positionInPath].IsDir {
			foundedIndex := -1
			for i, v := range *pathTree {
				if v.Name == pathParts[positionInPath].Name {
					foundedIndex = i
					break
				}
			}

			if foundedIndex == -1 { // Добавляем новую директорию
				newDir := models.Entry{
					IsDir:    true,
					Children: make([]models.Entry, 0),
					Name:     pathParts[positionInPath].Name,
				}
				recurseFileAdd(pathParts, positionInPath+1, &newDir.Children)
				*pathTree = append(*pathTree, newDir)
			} else { // Используем существующую
				recurseFileAdd(pathParts, positionInPath+1, &(*pathTree)[foundedIndex].Children)
			}
		} else { // Добавляем оконечный файл
			newFile := models.Entry{
				CreateAt: time.Time{},
				IsDir:    false,
				Size:     pathParts[positionInPath].Size,
				FilePath: pathParts[positionInPath].FilePath,
				Children: make([]models.Entry, 0),
				Name:     pathParts[positionInPath].Name,
			}
			*pathTree = append(*pathTree, newFile)
		}
	}
}
