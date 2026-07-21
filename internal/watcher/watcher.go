package watcher

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/kirsrus/iso2repo/models"
	"golang.org/x/exp/slog"
)

var _ models.Workerses = (*Watcher)(nil)

const (
	// DefaultChangeFiles размер буфера канала changeFiles по умолчанию.
	DefaultChangeFiles = 100000

	// DefaultPollInterval период опроса директории по умолчанию.
	DefaultPollInterval = 1 * time.Minute
)

// Watcher отслеживает состояние всех вложенных файлов в указанной директории.
// Передаёт через канал состояния обнаружения и потери файлов.
type Watcher struct {
	log *slog.Logger

	// Список файлов, которые уже отслежены.
	files []models.File

	rootDir string

	// Канал передачи события обнаружения/потери файлов.
	changeFiles chan<- models.FileEvent

	// Период опроса директории.
	pollInterval time.Duration
}

// Config конфигуркция конструктора структуры NewWatcher.
type Config struct {
	Log *slog.Logger

	// Канал передачи события обнаружения/потери файлов.
	ChangeFiles chan<- models.FileEvent

	// PollInterval период опроса директории. По умолчанию 1 минута.
	PollInterval time.Duration

	// Корневая дриектория с репозиториями.
	RootDir string
}

// NewWatcher конструктор структуры Watcher. RootDir - корневая директория, которая
// будет отслеживаться. Config описавает конфигурацию структуры.
func NewWatcher(config *Config) (*Watcher, error) {
	log := slog.New(slog.NewTextHandler(io.Discard))
	if config.Log != nil {
		log = config.Log
	}

	rootDir := strings.TrimSpace(config.RootDir)
	if rootDir == "" {
		return nil, errors.New("не заполнен RootDir")
	}

	if stat, err := os.Stat(rootDir); err != nil {
		return nil, errors.Errorf("RootDir недоступна: %w", err)
	} else if !stat.IsDir() {
		return nil, errors.Errorf("RootDir не является директорией: %s", rootDir)
	}

	changeFiles := make(chan<- models.FileEvent, DefaultChangeFiles)
	if config.ChangeFiles != nil {
		changeFiles = config.ChangeFiles
	}

	pollInterval := DefaultPollInterval
	if config.PollInterval > 0 {
		pollInterval = config.PollInterval
	}

	m := &Watcher{
		log:          log.With(slog.String("module", "watcher")),
		files:        make([]models.File, 0),
		rootDir:      rootDir,
		changeFiles:  changeFiles,
		pollInterval: pollInterval,
	}

	return m, nil
}

// Run запускает отслеживание файлов. Блокирующий.
// Завершается при отмене контекста или ошибке.
// Первая итерация выполняется сразу при запуске, последующие — с периодичностью pollInterval.
// Следующая итерация начинается только после завершения предыдущей.
func (m *Watcher) Run(ctx context.Context) error {
	m.log.Debug("запуск отслеживания директории", slog.String("rootDir", m.rootDir), slog.Duration("pollInterval", m.pollInterval))

	// Первичная синхронизация при запуске.
	if err := m.syncFiles(ctx); err != nil {
		// Если контекст отменён — штатное завершение.
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			m.log.Debug("отслеживание директории остановлено до первой синхронизации")

			return nil
		}
		return errors.Errorf("первичная синхронизация: %w", err)
	}

	// Таймер для следующей итерации.
	timer := time.NewTimer(m.pollInterval)
	defer timer.Stop()

	for {
		select {
		case <-timer.C:
			err := m.syncFiles(ctx)
			if err != nil {
				// Если ошибка связана с отменой контекста — выходим штатно.
				if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
					m.log.Debug("отслеживание директории остановлено")

					return nil
				}
				// Ошибка переполнения канала не фатальна — логируем и продолжаем.
				m.log.Warn("ошибка синхронизации, будет повтор на следующей итерации", slog.Any("error", err))
			}
			// Сброс таймера после завершения итерации.
			timer.Reset(m.pollInterval)

		case <-ctx.Done():
			m.log.Debug("отслеживание директории остановлено по сигналу")

			return nil
		}
	}
}

// collectFiles рекурсивно обходит rootDir и собирает абсолютные пути всех файлов.
// Если директория или файл недоступны по правам, они пропускаются с предупреждением в логе.
// Возвращается массив с абсолютном путём в файлу в ключе.
func (m *Watcher) collectFiles() (map[string]struct{}, error) {
	files := make(map[string]struct{})

	err := filepath.WalkDir(m.rootDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			// Если файл/директория недоступны — пропускаем с предупреждением.
			m.log.Warn("пропуск недоступного пути", slog.String("path", path), slog.Any("error", err))
			return nil
		}

		// Пропускаем директории, нас интересуют только файлы.
		if d.IsDir() {
			return nil
		}

		// Преобразуем в абсолютный путь (на всякий случай, WalkDir и так отдаёт абсолютный,
		// если rootDir абсолютный, но гарантируем).
		absPath, err := filepath.Abs(path)
		if err != nil {
			m.log.Warn("не удалось получить абсолютный путь", slog.String("path", path), slog.Any("error", err))
			return nil
		}

		files[absPath] = struct{}{}
		return nil
	})

	if err != nil {
		return nil, errors.Errorf("ошибка рекурсивного обхода директории %s: %w", m.rootDir, err)
	}

	return files, nil
}

// syncFiles сравнивает текущее состояние директории с отслеживаемым списком files.
// Сначала проверяет, какие отслеживаемые файлы пропали, затем — какие появились новые.
// Если событие не удалось отправить в канал (контекст отменён), files не изменяется —
// изменения будут применены при следующей итерации.
func (m *Watcher) syncFiles(ctx context.Context) error {
	m.log.Debug("начался процесс синхрнизации", slog.String("rootDir", m.rootDir))

	// Рекурсивно собираем все текущие файлы в rootDir.
	currentFiles, err := m.collectFiles()
	if err != nil {
		return err
	}

	// Строим множество отслеживаемых файлов для быстрого поиска.
	trackedFiles := make(map[string]int, len(m.files))
	for i, f := range m.files {
		trackedFiles[f.Path] = i
	}

	// 1. Проверяем, какие отслеживаемые файлы пропали из директории.
	//    Собираем пути пропавших файлов, но не изменяем files до успешной отправки.
	var lostFiles []string
	for _, f := range m.files {
		if _, exists := currentFiles[f.Path]; !exists {
			lostFiles = append(lostFiles, f.Path)
		}
	}

	// Отправляем события о пропавших файлах.
	for _, path := range lostFiles {
		err := m.sendEvent(ctx, models.FileEvent{
			File: models.File{
				Name: filepath.Base(path),
				Path: path,
			},
			EventType: models.FileLost,
		})
		if err != nil {
			// Не удалось отправить — выходим, files не меняем.
			return err
		}
		// Успешно отправили — удаляем из отслеживаемых.
		for i, f := range m.files {
			if f.Path == path {
				m.files = append(m.files[:i], m.files[i+1:]...)
				break
			}
		}

		m.log.Debug("файл потерян", slog.String("path", path))
	}

	// 2. Проверяем, какие новые файлы появились в директории.
	//    Собираем пути новых файлов, но не изменяем files до успешной отправки.
	var foundFiles []string
	for absPath := range currentFiles {
		if _, tracked := trackedFiles[absPath]; !tracked {
			foundFiles = append(foundFiles, absPath)
		}
	}

	// Отправляем события о найденных файлах.
	for _, path := range foundFiles {
		err := m.sendEvent(ctx, models.FileEvent{
			File: models.File{
				Name: filepath.Base(path),
				Path: path,
			},
			EventType: models.FileFound,
		})
		if err != nil {
			// Не удалось отправить — выходим, files не меняем.
			return err
		}
		// Успешно отправили — добавляем в отслеживаемые.
		m.files = append(m.files, models.File{
			Name: filepath.Base(path),
			Path: path,
		})
		m.log.Debug("файл обнаружен", slog.String("path", path))
	}

	if len(foundFiles) != 0 || len(lostFiles) != 0 {
		m.log.Info(fmt.Sprintf("обнаружено %d новых, %d удалённых файлов", len(foundFiles), len(lostFiles)))
	}

	return nil
}

// sendEvent безопасно отправляет событие в канал changeFiles с учётом контекста.
// Если канал переполнен, событие не отправляется и возвращается ошибка,
// чтобы вызывающий код не изменял files и повторил попытку на следующей итерации.
// Это предотвращает deadlock при заблокированном читателе.
func (m *Watcher) sendEvent(ctx context.Context, event models.FileEvent) error {
	select {
	case m.changeFiles <- event:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	default:
		// Канал переполнен — не блокируемся, вернём ошибку.
		return errors.New("канал changeFiles переполнен")
	}
}
