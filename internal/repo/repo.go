package repo

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"sync"

	"github.com/cockroachdb/errors"
	"github.com/kirsrus/iso2repo/models"
	"golang.org/x/exp/slog"
)

var _ models.Workerses = (*Repo)(nil)

const (
	// DefaultChangeFiles размер буфера канала changeFiles по умолчанию.
	DefaultChangeFiles = 100000

	// DefaultChangeRepos размер буфера канала changeRepos по умолчанию.
	DefaultChangeRepos = 100000
)

// Repo структура обнаружения, создания и удаления репозиториев и мехоанизмов
// их обработки.
type Repo struct {
	log *slog.Logger

	// Список репозиториев, которые уже отслежены.
	// Ключ — имя репозитория, значение — models.Repoes.
	repos sync.Map

	// Канал получения события обнаружения/потери файлов.
	changeFiles <-chan models.FileEvent

	// Канал передачи события обнаружения/порте репозиториев.
	changeRepos chan<- models.RepoEvent
}

// Config конфигурирует конструктор NewRepo.
type Config struct {
	Log *slog.Logger

	// Канал получения события обнаружения/потери файлов.
	ChangeFiles <-chan models.FileEvent

	// Канал передачи события обнаружения/порте репозиториев.
	ChangeRepos chan<- models.RepoEvent
}

// Newrepo конструктор Repo.
func NewRepo(config *Config) (*Repo, error) {
	log := slog.New(slog.NewTextHandler(io.Discard))
	if config.Log != nil {
		log = config.Log
	}

	changeFiles := make(<-chan models.FileEvent, DefaultChangeFiles)
	if config.ChangeFiles != nil {
		changeFiles = config.ChangeFiles
	}

	changeRepos := make(chan<- models.RepoEvent, DefaultChangeRepos)
	if config.ChangeRepos != nil {
		changeRepos = config.ChangeRepos
	}

	m := &Repo{
		log:         log.With(slog.String("module", "repo")),
		changeFiles: changeFiles,
		changeRepos: changeRepos,
	}

	return m, nil
}

// Run запускает отслеживание манипуляций с репозиторями. Блокирующий.
func (m *Repo) Run(ctx context.Context) error {
	m.log.Debug("запуск отслеживания репозиториев")

	for {
		select {
		// Пришло событие об изменении состояния файла в отслеживаемой
		// папке, т.е. файл или появился, или его удалили.
		case fileEvent := <-m.changeFiles:
			err := m.syncRepos(ctx, fileEvent)
			if err != nil {
				// Если ошибка связана с отменой контекста — выходим штатно.
				if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
					m.log.Debug("отслеживание репозиториев остановлено")

					return nil
				}

				m.log.Warn(fmt.Sprintf("ошибка отслеживания репозитория %s: %s", fileEvent.File.Name, err.Error()))
			}

		case <-ctx.Done():
			m.log.Debug("отслеживание репозиториев остановлено по сигналу")

			return nil
		}

	}
}

// syncRepos получает на вход событие об изменении состояния файла. На этом основании
// он проверяет, является ли этот файл репозиторием (или входи в его состав) и или
// добавляет, или удаляет новый репозиторий.
func (m *Repo) syncRepos(ctx context.Context, fileEvent models.FileEvent) error {
	// Обработка добавляемого файла.
	if fileEvent.EventType == models.FileFound {
		// Если файл имеет суффикс .iso, проверяем, не создано ли ещё репозитория
		// на его основе.
		if m.repoAlreadyExist(fileEvent) { // Репозиторий уже существует, пропускаем.
			return nil
		}

		// Проверяем, это ли директория из распакованного реопзитория и создана ли она.
		existingRepo := m.findExistingDirRepo(fileEvent)
		if existingRepo != nil {
			// Репозиторий уже существует. Если это custom-репозиторий — обновляем его,
			// пересканируя директорию и отправляя событие об обновлении.
			if customRepo, ok := existingRepo.(*RepoCustom); ok {
				customRepo.Refresh()
				m.sendEvent(ctx, models.RepoEvent{
					Repo:      customRepo,
					EventType: models.RepoFound,
				})
				m.log.Info(fmt.Sprintf("обновлён репозиторий %s (типа пользовательской папки)", customRepo.Metadata().Name))
			}
			// Для RepoExtracted обновление не требуется — он читает файлы с диска
			// при каждом запросе List().
			return nil
		}

		// Проверяем, является ли это распакованным репозиторием.
		isoDir := m.iso2dirInPath(fileEvent.File.Path)
		if isoDir != "" { // В элементе пути найден в пути суффикс .iso
			// Определяем его пока как распакованный репозиторий. Провеяем. Если как репозиторий
			// не определяется, оставляем его как составной репозиторий.
			repoIsoDir := NewRepoExtracted(m.isoDirFullPath(fileEvent.File.Path, isoDir), m.log)
			if repoIsoDir.IsRepo() { // Репозиторий оказался распакованным ISO.
				m.repos.Store(repoIsoDir.Metadata().Name, repoIsoDir)
				m.sendEvent(ctx, models.RepoEvent{
					Repo:      repoIsoDir,
					EventType: models.RepoFound,
				})

				m.log.Info(fmt.Sprintf("обнаружен новый репозиторий %s (типа распакованного ISO)", repoIsoDir.Metadata().Name))

				return nil
			}

			// Репозиторий считаем составным, пользовательским репозиторием.
			repoDir := NewRepoCustom(m.isoDirFullPath(fileEvent.File.Path, isoDir), m.log)
			m.repos.Store(repoDir.Metadata().Name, repoDir)
			m.sendEvent(ctx, models.RepoEvent{
				Repo:      repoDir,
				EventType: models.RepoFound,
			})

			m.log.Info(fmt.Sprintf("обнаружен новый репозиторий %s (типа пользовательской папки)", repoIsoDir.Metadata().Name))

			return nil
		}

		// В составных частях пути не обнаружено .iso суффиксов. Проверяем на принадлженость и файлу .iso репозитория.
		if strings.HasSuffix(fileEvent.File.Name, ".iso") {
			repo, err := NewRepoIso(fileEvent.File.Path, m.log)
			if err != nil {
				return err
			}

			if !repo.IsRepo() { // Не является репозиторием
				return errors.New("iso-образ не является репозиторием")
			}

			m.repos.Store(repo.Metadata().Name, repo)
			m.sendEvent(ctx, models.RepoEvent{
				Repo:      repo,
				EventType: models.RepoFound,
			})

			m.log.Info(fmt.Sprintf("обнаружен новый репозиторий %s (типа iso-файла)", repo.Metadata().Name))

			return nil
		}

		m.log.Debug("обнаруженный файл пропущен", slog.String("path", fileEvent.File.Path))
		return nil
	}

	// Обработка удаляемого файла.
	if fileEvent.EventType == models.FileLost {
		// Проверяем, не находится ли удалённый файл внутри custom-репозитория.
		// Если да — обновляем репозиторий, пересканируя его содержимое.
		existingRepo := m.findExistingDirRepo(fileEvent)
		if existingRepo != nil {
			if customRepo, ok := existingRepo.(*RepoCustom); ok {
				customRepo.Refresh()
				m.sendEvent(ctx, models.RepoEvent{
					Repo:      customRepo,
					EventType: models.RepoFound,
				})
				m.log.Info(fmt.Sprintf("обновлён репозиторий %s после удаления файла (типа пользовательской папки)", customRepo.Metadata().Name))
			}
			// Для RepoExtracted обновление не требуется — он читает файлы с диска
			// при каждом запросе List().
			return nil
		}

		// Проверяем, является ли удалённый файл ISO-образом.
		if !strings.HasSuffix(fileEvent.File.Name, ".iso") {
			m.log.Debug("удалённый файл пропущен", slog.String("path", fileEvent.File.Path))
			return nil
		}

		// Ищем репозиторий по имени файла и удаляем его.
		if value, loaded := m.repos.LoadAndDelete(fileEvent.File.Name); loaded {
			repo, ok := value.(models.Repoes)
			if ok {
				m.sendEvent(ctx, models.RepoEvent{
					Repo:      repo,
					EventType: models.RepoLost,
				})
			}

			m.log.Info("удалён iso-репозиторий", slog.String("repo", fileEvent.File.Name))
		} else {
			m.log.Debug("репозиторий для удалённого файла не найден", slog.String("path", fileEvent.File.Path))
		}

		return nil
	}

	return errors.New("not implement")
}

// repoAlreadyExist проверяет в локальной базе наличие репозитория на основе файла, полученного из fileEvent.
func (m *Repo) repoAlreadyExist(fileEvent models.FileEvent) bool {
	found := false

	m.repos.Range(func(_, value any) bool {
		repo, _ := value.(models.Repoes)

		if repo.Metadata().Type == models.RepoISO && repo.Metadata().Name == fileEvent.File.Name {
			found = true

			return false // прерываем обход
		}

		return true // продолжаем обход
	})

	return found
}

// findExistingDirRepo проверяет, не является ли добавленный файл уже в составе директории одного из составных репозиториев
// и создан ли на его основе уже репозиторий. Если да — возвращает найденный репозиторий, иначе nil.
func (m *Repo) findExistingDirRepo(fileEvent models.FileEvent) models.Repoes {
	iso2dir := m.iso2dirInPath(fileEvent.File.Path)
	if iso2dir == "" {
		return nil
	}

	var found models.Repoes

	m.repos.Range(func(_, value any) bool {
		repo, _ := value.(models.Repoes)

		if (repo.Metadata().Type == models.RepoExtracted || repo.Metadata().Type == models.RepoCustom) && iso2dir == repo.Metadata().Name {
			found = repo

			return false // прерываем обход
		}

		return true // продолжаем обход
	})

	return found
}

// iso2dirInPath находит первое вхождение имени директории с суфиксом ".iso" в путь path.
// Если директория обнаружена, она возвращается в ответе, если нет, возвращается пустая строка.
func (m *Repo) iso2dirInPath(path string) string {
	dir := filepath.Dir(filepath.Clean(path))
	parts := strings.Split(dir, string(filepath.Separator))
	for _, part := range parts {
		if strings.HasSuffix(part, ".iso") {
			return part
		}
	}
	return ""
}

// isoDirFullPath получает полный путь к директории isoDir из абсолютного пути
// path.
func (m *Repo) isoDirFullPath(path string, isoDir string) string {
	before, _, ok := strings.Cut(path, isoDir)
	if !ok {
		return ""
	}

	return before + isoDir
}

// sendEvent безопасно отправляет событие в канал changeRepos.
func (m *Repo) sendEvent(ctx context.Context, event models.RepoEvent) error {
	select {
	case m.changeRepos <- event:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	default:
		// Канал переполнен — не блокируемся, вернём ошибку.
		return errors.New("канал changeRepos переполнен")
	}
}
