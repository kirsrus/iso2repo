package repo

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"sort"
	"strings"

	"github.com/kirsrus/iso2repo/models"
	"github.com/kirsrus/iso2repo/pkg/sevenz"
)

var _ models.Repoes = (*RepoIso)(nil)

type RepoIso struct {
	log      *slog.Logger
	name     string
	path     string
	repoType models.RepoType
	sevenZ   *sevenz.SevenZ

	// Кэшированный список файлов в образе. Считывается из ISO
	// только при первом обращении, а затем хранится только здесь.
	cacheISOFiles []models.Entry

	// Индикатор заполненности кэша cacheISOFiles.
	cacheISOFilesIsFull bool
}

func NewRepoIso(fullPath string, log *slog.Logger) (*RepoIso, error) {
	var err error

	if log == nil {
		log = slog.New(slog.NewTextHandler(io.Discard, nil))
	}

	// Создаём экземпляр SevenZ для работы с утилитой 7z
	sevenZ, err := sevenz.NewSevenZ(log)
	if err != nil {
		return nil, err
	}

	m := &RepoIso{
		log:      log.With("sub", "iso"),
		name:     filepath.Base(fullPath),
		path:     fullPath,
		repoType: models.RepoISO,
		sevenZ:   sevenZ,
	}

	return m, nil
}

func (m *RepoIso) Metadata() models.Repo {
	return models.Repo{
		Name: m.name,
		Path: m.path,
		Type: m.repoType,
	}
}

func (m *RepoIso) IsRepo() bool {
	if len(m.cacheISOFiles) == 0 {
		files, err := m.sevenZ.ListFiles(m.path)
		if err != nil {
			return false
		}
		m.cacheISOFiles = files
	}

	// Проверяем, что диск является дистрибутивом.
	// Для этого ищем в корне папку /dists, и если в ней есть подпапки и хотя бы
	// в одной подпапке есть файл Release — это репозиторий.

	dists, err := m.List(context.Background(), "/dists")
	if err != nil {
		m.log.Warn("iso-образ не является репозиторием")

		return false
	}

	for _, dir := range dists {
		if !dir.IsDir {
			continue
		}
		for _, child := range dir.Children {
			if child.Name == "Release" {
				return true
			}
		}
	}

	return false
}

// RepoString возвращает строковое представление данных в репозитории для клиентов.
// Например:
//
//	deb [arch=amd64] http://repo.loc:4309/repo/1.7_loader.iso 1.7_x86-64 contrib main non-free
//
// Алгоритм работы:
//  1. Ищем в корне ISO каталог /dists.
//  2. Внутри /dists ищем первую поддиректорию — это имя дистрибутива (например, "1.7_x86-64").
//  3. Внутри директории дистрибутива ищем файл "Release".
//  4. Читаем файл Release, ищем строку с префиксом "components:".
//  5. Извлекаем список компонентов (contrib, main, non-free и т.д.).
//  6. Проверяем, что для каждого компонента существует соответствующая поддиректория
//     внутри /dists/<distributeName>/.
//  7. Сортируем компоненты и формируем итоговую строку.
//
// Если на любом этапе данные не найдены или произошла ошибка, возвращается пустая строка.
// Результат возвращается в формате "deb http://0.0.0.0/repo/%s %s %s"
func (m *RepoIso) RepoString() string {
	// Убеждаемся, что кэш файлов загружен
	if !m.cacheISOFilesIsFull {
		files, err := m.sevenZ.ListFiles(m.path)
		if err != nil {
			m.log.Warn("не удалось прочитать список файлов ISO", slog.String("error", err.Error()))
			return ""
		}
		m.cacheISOFiles = files
		m.cacheISOFilesIsFull = true
	}

	// 1. Получаем содержимое каталога /dists
	dists, err := m.List(context.Background(), "/dists")
	if err != nil {
		m.log.Warn("не удалось прочитать /dists", slog.String("error", err.Error()))
		return ""
	}

	// 2. Ищем первую директорию внутри /dists — это имя дистрибутива
	distributeName := ""
	for _, v := range dists {
		if v.IsDir {
			distributeName = v.Name
			break
		}
	}
	if distributeName == "" {
		m.log.Warn("в /dists не найдено ни одной поддиректории")
		return ""
	}

	// 3. Ищем файл Release внутри /dists/<distributeName>
	distEntries, err := m.List(context.Background(), "/dists/"+distributeName)
	if err != nil {
		m.log.Warn("не удалось прочитать /dists/"+distributeName, slog.String("error", err.Error()))
		return ""
	}

	releaseFound := false
	for _, v := range distEntries {
		if v.Name == "Release" && !v.IsDir {
			releaseFound = true
			break
		}
	}
	if !releaseFound {
		m.log.Warn("в /dists/" + distributeName + " не найден файл Release")
		return ""
	}

	// 4. Читаем содержимое файла Release
	releasePath := "/dists/" + distributeName + "/Release"
	reader, err := m.sevenZ.Open(context.Background(), m.path, releasePath)
	if err != nil {
		m.log.Warn("не удалось открыть файл Release", slog.String("error", err.Error()))
		return ""
	}
	defer reader.Close()

	releaseData, err := io.ReadAll(reader)
	if err != nil {
		m.log.Warn("не удалось прочитать файл Release", slog.String("error", err.Error()))
		return ""
	}

	// 5. Ищем строку с префиксом "components:"
	findPrefix := "components:"
	components := make([]string, 0)
	for _, line := range strings.Split(string(releaseData), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(strings.ToLower(line), findPrefix) {
			// Убираем префикс и разбиваем остаток по пробелам/табуляциям
			lineClean := strings.ReplaceAll(line, "\t", " ")
			linePart := strings.Split(lineClean[len(findPrefix):], " ")
			for _, p := range linePart {
				pClean := strings.TrimSpace(p)
				if pClean == "" {
					continue
				}
				// 6. Проверяем, что для компонента существует директория
				//    /dists/<distributeName>/<component>
				compEntries, err := m.List(context.Background(), "/dists/"+distributeName+"/"+pClean)
				if err != nil || len(compEntries) == 0 {
					// Директория не найдена — пропускаем компонент
					continue
				}
				// Проверяем, не дубликат ли это
				alreadyExists := false
				for _, c := range components {
					if c == pClean {
						alreadyExists = true
						break
					}
				}
				if !alreadyExists {
					components = append(components, pClean)
				}
			}
			break
		}
	}

	if len(components) == 0 {
		m.log.Warn(fmt.Sprintf("в файле '%s' не найден блок '%s'", releasePath, findPrefix))
		return ""
	}

	// 7. Сортируем компоненты и формируем результат
	sort.Strings(components)
	result := fmt.Sprintf("deb [arch=amd64] http://0.0.0.0/repo/%s %s %s", filepath.Base(m.path), distributeName, strings.Join(components, " "))

	return result
}

// List возвращает список записей по указанному пути внутри образа.
// Путь должен быть относительным корня образа, без ведущего "/".
// Для корневого каталога передаётся пустая строка.
func (m *RepoIso) List(ctx context.Context, path string) ([]models.Entry, error) {
	if !m.cacheISOFilesIsFull {
		var err error
		m.cacheISOFiles, err = m.sevenZ.ListFiles(m.path)

		if err != nil {
			return []models.Entry{}, err
		}
		m.cacheISOFilesIsFull = true
	}

	// Нормализуем путь: убираем ведущий и завершающий слеши
	path = strings.Trim(path, "/")
	if path == "" {
		// Корневой каталог — возвращаем всё дерево
		return m.cacheISOFiles, nil
	}

	// Разбиваем путь на сегменты и рекурсивно ищем нужную директорию
	segments := strings.Split(path, "/")
	entries := m.cacheISOFiles

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
			// Директория не найдена — возвращаем пустой результат
			return []models.Entry{}, nil
		}
	}

	return entries, nil
}

// Open открывает файл внутри ISO для потокового чтения.
// При отмене ctx процесс 7z принудительно завершается.
func (m *RepoIso) Open(ctx context.Context, path string) (io.ReadCloser, error) {
	return m.sevenZ.Open(ctx, m.path, path)
}
