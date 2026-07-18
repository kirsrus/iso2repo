package repo

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/kirsrus/iso2repo/models"
	"github.com/kirsrus/iso2repo/pkg/deb"
)

var _ models.Repoes = (*RepoCustom)(nil)

// debFileInfo хранит информацию о .deb файле для генерации Packages.
type debFileInfo struct {
	Name      string
	Path      string
	Size      int64
	MD5Sum    string
	SHA1Sum   string
	SHA256Sum string
	FileTime  time.Time
	Meta      *deb.PackageMeta // Метаданные из control-файла .deb пакета
}

// RepoCustom эмулирует работу apt-репозитория на основе директории с .deb файлами.
// Динамически создаёт виртуальную структуру:
//
//	dists/
//	  custom/
//	    Release
//	    main/
//	      binary-amd64/
//	        Packages
//	pool/
//	  main/
//	    <файлы>.deb
//
// Release и Packages генерируются на основе реальных .deb файлов в директории.
type RepoCustom struct {
	log      *slog.Logger
	name     string
	path     string
	repoType models.RepoType

	// Мьютекс для защиты кэша при параллельных запросах
	mu sync.RWMutex

	// Кэшированная древовидная структура файлов репозитория
	cacheFiles []models.Entry

	// Индикатор заполненности кэша
	cacheFilesIsFull bool

	// Список .deb файлов с их метаданными
	debFiles []debFileInfo

	// Время запуска программы (фиксируется при создании репозитория)
	startTime time.Time

	// Сгенерированное содержимое Release файла
	releaseContent []byte

	// Сгенерированное содержимое Packages файла
	packagesContent []byte
}

// NewRepoCustom конструктор RepoCustom.
// fullPath — абсолютный путь к директории с .deb файлами.
func NewRepoCustom(fullPath string, log *slog.Logger) *RepoCustom {
	if log == nil {
		log = slog.New(slog.NewTextHandler(io.Discard, nil))
	}

	m := &RepoCustom{
		log:       log.With("sub", "custom"),
		name:      filepath.Base(fullPath),
		path:      fullPath,
		repoType:  models.RepoCustom,
		startTime: time.Now().UTC(),
	}

	// При создании сразу сканируем директорию и генерируем структуру
	m.scanDebFiles()

	return m
}

// Refresh повторно сканирует директорию репозитория, обновляя список .deb файлов
// и перегенерируя Packages и Release. Используется когда в уже существующий
// репозиторий добавляются новые файлы.
func (m *RepoCustom) Refresh() {
	m.log.Debug("обновление custom-репозитория", slog.String("name", m.name))
	m.scanDebFiles()
}

func (m *RepoCustom) Metadata() models.Repo {
	return models.Repo{
		Name: m.name,
		Path: m.path,
		Type: m.repoType,
	}
}

// IsRepo всегда возвращает true, так как RepoCustom по определению является репозиторием.
func (m *RepoCustom) IsRepo() bool {
	return true
}

// RepoString возвращает строковое представление данных в репозитории для клиентов.
// Например:
//
//	deb [arch=amd64] http://repo.loc:4309/repo/custom-repo.iso custom contrib main non-free
func (m *RepoCustom) RepoString() string {
	return fmt.Sprintf("deb [arch=amd64 trusted=yes] http://0.0.0.0/repo/%s custom contrib main non-free", m.name)
}

// List возвращает список записей по указанному пути внутри виртуального репозитория.
// Путь должен быть относительным корня репозитория, без ведущего "/".
// Для корневого каталога передаётся пустая строка.
func (m *RepoCustom) List(ctx context.Context, path string) ([]models.Entry, error) {
	m.mu.RLock()
	if !m.cacheFilesIsFull {
		m.mu.RUnlock()
		// Перезахватываем с записью для инициализации
		m.mu.Lock()
		if !m.cacheFilesIsFull {
			m.buildCache()
		}
		m.mu.Unlock()
		m.mu.RLock()
	}
	defer m.mu.RUnlock()

	// Нормализуем путь: убираем ведущий и завершающий слеши
	path = strings.Trim(path, "/")
	if path == "" {
		return m.cacheFiles, nil
	}

	// Разбиваем путь на сегменты и рекурсивно ищем нужную директорию
	segments := strings.Split(path, "/")
	entries := m.cacheFiles

	for _, segment := range segments {
		found := false
		for _, entry := range entries {
			if entry.IsDir && entry.Name == segment {
				entries = entry.Children
				found = true
				break
			}
		}
		if !found {
			return []models.Entry{}, nil
		}
	}

	return entries, nil
}

// Open открывает файл внутри виртуального репозитория для потокового чтения.
// Поддерживает как реальные .deb файлы, так и сгенерированные Release/Packages.
func (m *RepoCustom) Open(ctx context.Context, path string) (io.ReadCloser, error) {
	path = strings.Trim(path, "/")

	// Проверяем, не запрашивается ли сгенерированный файл
	switch path {
	case "dists/custom/Release":
		return io.NopCloser(bytes.NewReader(m.releaseContent)), nil
	case "dists/custom/main/binary-amd64/Packages":
		return io.NopCloser(bytes.NewReader(m.packagesContent)), nil
	}

	// Иначе — ищем .deb файл
	// Путь может быть вида: pool/main/<filename>.deb
	// Ищем по имени файла в debFiles
	fileName := filepath.Base(path)
	for _, deb := range m.debFiles {
		if deb.Name == fileName {
			file, err := os.Open(deb.Path)
			if err != nil {
				return nil, err
			}
			return file, nil
		}
	}

	return nil, fmt.Errorf("файл не найден: %s", path)
}

// scanDebFiles сканирует директорию репозитория и собирает информацию о .deb файлах.
func (m *RepoCustom) scanDebFiles() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.debFiles = make([]debFileInfo, 0)

	err := filepath.WalkDir(m.path, func(currentPath string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Пропускаем директории
		if d.IsDir() {
			return nil
		}

		// Интересуемся только .deb файлами
		if !strings.HasSuffix(strings.ToLower(d.Name()), ".deb") {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			m.log.Warn("не удалось получить информацию о файле", slog.String("file", currentPath))
			return nil
		}

		// Вычисляем MD5, SHA1 и SHA256 хэши файла за один проход
		md5Sum, sha1Sum, sha256Sum, err := m.computeHashes(currentPath)
		if err != nil {
			m.log.Warn("не удалось вычислить хэши", slog.String("file", currentPath), slog.Any("error", err))
		}

		// Извлекаем метаданные из .deb пакета
		meta, err := deb.ExtractMeta(currentPath)
		if err != nil {
			m.log.Warn("не удалось извлечь метаданные из .deb", slog.String("file", currentPath), slog.Any("error", err))
			meta = nil
		}

		m.debFiles = append(m.debFiles, debFileInfo{
			Name:      d.Name(),
			Path:      currentPath,
			Size:      info.Size(),
			MD5Sum:    md5Sum,
			SHA1Sum:   sha1Sum,
			SHA256Sum: sha256Sum,
			FileTime:  info.ModTime(),
			Meta:      meta,
		})

		return nil
	})

	if err != nil {
		m.log.Warn("ошибка сканирования директории", slog.String("path", m.path), slog.Any("error", err))
	}

	// Сортируем .deb файлы по имени для стабильного вывода
	sort.Slice(m.debFiles, func(i, j int) bool {
		return m.debFiles[i].Name < m.debFiles[j].Name
	})

	m.log.Debug("сканирование завершено", slog.Int("deb_files", len(m.debFiles)))

	// Генерируем содержимое Packages и Release (важен порядок: сначала Packages, потом Release)
	m.generatePackagesContent()
	m.generateReleaseContent()

	// Инвалидируем кэш дерева
	m.cacheFilesIsFull = false
}

// computeHashes вычисляет MD5, SHA1 и SHA256 хэши файла за один проход чтения.
func (m *RepoCustom) computeHashes(filePath string) (md5Str, sha1Str, sha256Str string, err error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", "", "", err
	}
	defer file.Close()

	md5H := md5.New()
	sha1H := sha1.New()
	sha256H := sha256.New()

	// MultiWriter пишет во все хэши одновременно за один проход
	multiWriter := io.MultiWriter(md5H, sha1H, sha256H)
	if _, err := io.Copy(multiWriter, file); err != nil {
		return "", "", "", err
	}

	return hex.EncodeToString(md5H.Sum(nil)),
		hex.EncodeToString(sha1H.Sum(nil)),
		hex.EncodeToString(sha256H.Sum(nil)),
		nil
}

// generateReleaseContent генерирует содержимое файла Release.
func (m *RepoCustom) generateReleaseContent() {
	now := m.startTime.Format("Mon, 02 Jan 2006 15:04:05 MST")

	var buf bytes.Buffer
	fmt.Fprintf(&buf, "Origin: Custom\n")
	fmt.Fprintf(&buf, "Suite: stable\n")
	fmt.Fprintf(&buf, "Label: Custom apt repository\n")
	fmt.Fprintf(&buf, "Codename: custom\n")
	fmt.Fprintf(&buf, "Date: %s\n", now)
	fmt.Fprintf(&buf, "Architectures: amd64\n")
	fmt.Fprintf(&buf, "Components: main contrib non-free\n")
	fmt.Fprintf(&buf, "Description: Custom apt repository\n")

	packagesSize := len(m.packagesContent)

	// MD5Sum
	fmt.Fprintf(&buf, "MD5Sum:\n")
	if packagesSize > 0 {
		packagesMD5 := fmt.Sprintf("%x", md5.Sum(m.packagesContent))
		fmt.Fprintf(&buf, " %s %d main/binary-amd64/Packages\n", packagesMD5, packagesSize)
	}

	// SHA1
	fmt.Fprintf(&buf, "SHA1:\n")
	if packagesSize > 0 {
		packagesSHA1 := fmt.Sprintf("%x", sha1.Sum(m.packagesContent))
		fmt.Fprintf(&buf, " %s %d main/binary-amd64/Packages\n", packagesSHA1, packagesSize)
	}

	// SHA256
	fmt.Fprintf(&buf, "SHA256:\n")
	if packagesSize > 0 {
		packagesSHA256 := fmt.Sprintf("%x", sha256.Sum256(m.packagesContent))
		fmt.Fprintf(&buf, " %s %d main/binary-amd64/Packages\n", packagesSHA256, packagesSize)
	}

	m.releaseContent = buf.Bytes()
}

// generatePackagesContent генерирует содержимое файла Packages на основе .deb файлов.
func (m *RepoCustom) generatePackagesContent() {
	var buf bytes.Buffer

	for _, deb := range m.debFiles {
		// Имя пакета — имя файла без расширения .deb (запасной вариант)
		pkgName := strings.TrimSuffix(deb.Name, ".deb")

		// Используем метаданные из .deb пакета, если они доступны
		if deb.Meta != nil {
			fmt.Fprintf(&buf, "Package: %s\n", deb.Meta.Package)
			if deb.Meta.Version != "" {
				fmt.Fprintf(&buf, "Version: %s\n", deb.Meta.Version)
			}
			if deb.Meta.Architecture != "" {
				fmt.Fprintf(&buf, "Architecture: %s\n", deb.Meta.Architecture)
			}
			if deb.Meta.Maintainer != "" {
				fmt.Fprintf(&buf, "Maintainer: %s\n", deb.Meta.Maintainer)
			}
			if deb.Meta.InstalledSize != "" {
				fmt.Fprintf(&buf, "Installed-Size: %s\n", deb.Meta.InstalledSize)
			}
			if deb.Meta.Depends != "" {
				fmt.Fprintf(&buf, "Depends: %s\n", deb.Meta.Depends)
			}
			if deb.Meta.PreDepends != "" {
				fmt.Fprintf(&buf, "Pre-Depends: %s\n", deb.Meta.PreDepends)
			}
			if deb.Meta.Recommends != "" {
				fmt.Fprintf(&buf, "Recommends: %s\n", deb.Meta.Recommends)
			}
			if deb.Meta.Suggests != "" {
				fmt.Fprintf(&buf, "Suggests: %s\n", deb.Meta.Suggests)
			}
			if deb.Meta.Conflicts != "" {
				fmt.Fprintf(&buf, "Conflicts: %s\n", deb.Meta.Conflicts)
			}
			if deb.Meta.Replaces != "" {
				fmt.Fprintf(&buf, "Replaces: %s\n", deb.Meta.Replaces)
			}
			if deb.Meta.Provides != "" {
				fmt.Fprintf(&buf, "Provides: %s\n", deb.Meta.Provides)
			}
			if deb.Meta.Section != "" {
				fmt.Fprintf(&buf, "Section: %s\n", deb.Meta.Section)
			}
			if deb.Meta.Priority != "" {
				fmt.Fprintf(&buf, "Priority: %s\n", deb.Meta.Priority)
			}
			if deb.Meta.Homepage != "" {
				fmt.Fprintf(&buf, "Homepage: %s\n", deb.Meta.Homepage)
			}
			// Дополнительные поля из Extra
			for k, v := range deb.Meta.Extra {
				fmt.Fprintf(&buf, "%s: %s\n", k, v)
			}
		} else {
			// Если метаданные недоступны, используем запасные значения
			fmt.Fprintf(&buf, "Package: %s\n", pkgName)
			fmt.Fprintf(&buf, "Version: 1.0\n")
			fmt.Fprintf(&buf, "Architecture: amd64\n")
			fmt.Fprintf(&buf, "Maintainer: Custom Repository\n")
		}

		fmt.Fprintf(&buf, "Filename: pool/main/%s\n", deb.Name)
		fmt.Fprintf(&buf, "Size: %d\n", deb.Size)
		if deb.MD5Sum != "" {
			fmt.Fprintf(&buf, "MD5sum: %s\n", deb.MD5Sum)
		}
		if deb.SHA1Sum != "" {
			fmt.Fprintf(&buf, "SHA1: %s\n", deb.SHA1Sum)
		}
		if deb.SHA256Sum != "" {
			fmt.Fprintf(&buf, "SHA256: %s\n", deb.SHA256Sum)
		}
		// Если Description не был выведен из метаданных, добавляем запасной
		if deb.Meta != nil && deb.Meta.Description == "" {
			fmt.Fprintf(&buf, "Description: %s\n", pkgName)
		} else if deb.Meta == nil {
			fmt.Fprintf(&buf, "Description: %s\n", pkgName)
		}
		buf.WriteString("\n")
	}

	m.packagesContent = buf.Bytes()
}

// buildCache строит древовидную структуру виртуального репозитория.
func (m *RepoCustom) buildCache() {
	m.cacheFiles = make([]models.Entry, 0)

	// Строим виртуальную структуру:
	// dists/
	//   custom/
	//     Release (файл)
	//     main/
	//       binary-amd64/
	//         Packages (файл)
	// pool/
	//   main/
	//     <deb файлы>

	// Создаём dists/custom/Release
	releaseEntry := models.Entry{
		Name:     "Release",
		IsDir:    false,
		Size:     int64(len(m.releaseContent)),
		CreateAt: m.startTime,
		Children: make([]models.Entry, 0),
	}

	// Создаём dists/custom/main/binary-amd64/Packages
	packagesEntry := models.Entry{
		Name:     "Packages",
		IsDir:    false,
		Size:     int64(len(m.packagesContent)),
		CreateAt: m.startTime,
		Children: make([]models.Entry, 0),
	}

	// Создаём dists/custom/main/binary-amd64/
	binaryAmd64Dir := models.Entry{
		Name:     "binary-amd64",
		IsDir:    true,
		Children: []models.Entry{packagesEntry},
	}

	// Создаём dists/custom/main/
	mainDir := models.Entry{
		Name:     "main",
		IsDir:    true,
		Children: []models.Entry{binaryAmd64Dir},
	}

	// Создаём dists/custom/
	customDir := models.Entry{
		Name:     "custom",
		IsDir:    true,
		Children: []models.Entry{releaseEntry, mainDir},
	}

	// Создаём dists/
	distsDir := models.Entry{
		Name:     "dists",
		IsDir:    true,
		Children: []models.Entry{customDir},
	}

	m.cacheFiles = append(m.cacheFiles, distsDir)

	// Создаём pool/main/ с .deb файлами
	poolMainDir := models.Entry{
		Name:     "main",
		IsDir:    true,
		Children: make([]models.Entry, 0),
	}

	for _, deb := range m.debFiles {
		debEntry := models.Entry{
			Name:     deb.Name,
			IsDir:    false,
			Size:     deb.Size,
			CreateAt: deb.FileTime,
			Children: make([]models.Entry, 0),
		}
		poolMainDir.Children = append(poolMainDir.Children, debEntry)
	}

	poolDir := models.Entry{
		Name:     "pool",
		IsDir:    true,
		Children: []models.Entry{poolMainDir},
	}

	m.cacheFiles = append(m.cacheFiles, poolDir)

	m.cacheFilesIsFull = true
}
