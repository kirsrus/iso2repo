package repo

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/kirsrus/iso2repo/models"
	"golang.org/x/exp/slog"
)

var _ models.Repoes = (*RepoExtracted)(nil)

type RepoExtracted struct {
	log      *slog.Logger
	name     string
	path     string
	repoType models.RepoType

	// Кэшированный список файлов в директории репозитория.
	// Считывается только при первом обращении, затем хранится здесь.
	cacheFiles []models.Entry

	// Индикатор заполненности кэша cacheFiles.
	cacheFilesIsFull bool
}

func NewRepoExtracted(fullPath string, log *slog.Logger) *RepoExtracted {
	if log == nil {
		log = slog.New(slog.NewTextHandler(io.Discard))
	}

	return &RepoExtracted{
		log:      log.With("sub", "extracted"),
		name:     filepath.Base(fullPath),
		path:     fullPath,
		repoType: models.RepoExtracted,
	}
}

func (m *RepoExtracted) Metadata() models.Repo {
	return models.Repo{
		Name: m.name,
		Path: m.path,
		Type: m.repoType,
	}
}

// IsRepo проверяет, является ли директория репозиторием.
// Для этого ищет в корне папку /dists, и если в ней есть подпапки и хотя бы
// в одной подпапке есть файл Release — это репозиторий.
func (m *RepoExtracted) IsRepo() bool {
	distsPath := filepath.Join(m.path, "dists")
	distsDir, err := os.ReadDir(distsPath)
	if err != nil {
		m.log.Debug("директория не является репозиторием", slog.String("path", m.path))
		return false
	}

	for _, dir := range distsDir {
		if !dir.IsDir() {
			continue
		}
		releasePath := filepath.Join(distsPath, dir.Name(), "Release")
		if _, err := os.Stat(releasePath); err == nil {
			return true
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
//  1. Ищем в корне каталог /dists.
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
func (m *RepoExtracted) RepoString() string {
	// 1. Получаем содержимое каталога /dists
	distsPath := filepath.Join(m.path, "dists")
	distsDir, err := os.ReadDir(distsPath)
	if err != nil {
		m.log.Warn("не удалось прочитать /dists", slog.String("error", err.Error()))
		return ""
	}

	// 2. Ищем первую директорию внутри /dists — это имя дистрибутива
	distributeName := ""
	for _, v := range distsDir {
		if v.IsDir() {
			distributeName = v.Name()
			break
		}
	}
	if distributeName == "" {
		m.log.Warn("в /dists не найдено ни одной поддиректории")
		return ""
	}

	// 3. Ищем файл Release внутри /dists/<distributeName>
	distPath := filepath.Join(distsPath, distributeName)
	distEntries, err := os.ReadDir(distPath)
	if err != nil {
		m.log.Warn("не удалось прочитать /dists/"+distributeName, slog.String("error", err.Error()))
		return ""
	}

	releaseFound := false
	for _, v := range distEntries {
		if !v.IsDir() && v.Name() == "Release" {
			releaseFound = true
			break
		}
	}
	if !releaseFound {
		m.log.Warn("в /dists/" + distributeName + " не найден файл Release")
		return ""
	}

	// 4. Читаем содержимое файла Release
	releasePath := filepath.Join(distPath, "Release")
	releaseData, err := os.ReadFile(releasePath)
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
				compPath := filepath.Join(distPath, pClean)
				compInfo, err := os.Stat(compPath)
				if err != nil || !compInfo.IsDir() {
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
	result := fmt.Sprintf("deb [arch=amd64] http://0.0.0.0/repo/%s %s %s", m.name, distributeName, strings.Join(components, " "))

	return result
}

// List возвращает список записей по указанному пути внутри директории репозитория.
// Путь должен быть относительным корня репозитория, без ведущего "/".
// Для корневого каталога передаётся пустая строка.
func (m *RepoExtracted) List(ctx context.Context, path string) ([]models.Entry, error) {
	if !m.cacheFilesIsFull {
		var err error
		m.cacheFiles, err = m.readFilesFromFS(m.path)
		if err != nil {
			return []models.Entry{}, err
		}
		m.cacheFilesIsFull = true
	}

	// Нормализуем путь: убираем ведущий и завершающий слеши
	path = strings.Trim(path, "/")
	if path == "" {
		// Корневой каталог — возвращаем всё дерево
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
			// Директория не найдена — возвращаем пустой результат
			return []models.Entry{}, nil
		}
	}

	return entries, nil
}

// Open открывает файл внутри директории репозитория для потокового чтения.
func (m *RepoExtracted) Open(ctx context.Context, path string) (io.ReadCloser, error) {
	filePath := filepath.Join(m.path, path)

	// Проверяем, что файл находится внутри директории репозитория (безопасность)
	absRepo, _ := filepath.Abs(m.path)
	absFile, _ := filepath.Abs(filePath)
	if !strings.HasPrefix(absFile, absRepo) {
		return nil, fmt.Errorf("путь выходит за пределы репозитория: %s", path)
	}

	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}

	return file, nil
}

// readFilesFromFS рекурсивно читает содержимое директории и строит
// древовидную структуру []models.Entry.
func (m *RepoExtracted) readFilesFromFS(rootPath string) ([]models.Entry, error) {
	result := make([]models.Entry, 0)

	err := filepath.WalkDir(rootPath, func(currentPath string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Пропускаем корневую директорию
		if currentPath == rootPath {
			return nil
		}

		// Получаем относительный путь от корня репозитория
		relPath, err := filepath.Rel(rootPath, currentPath)
		if err != nil {
			return err
		}

		// Нормализуем разделители путей
		relPath = filepath.ToSlash(relPath)

		info, err := d.Info()
		if err != nil {
			return err
		}

		newEntry := models.Entry{
			Name:     d.Name(),
			IsDir:    d.IsDir(),
			Size:     info.Size(),
			CreateAt: info.ModTime(),
			Children: make([]models.Entry, 0),
		}

		// Разбиваем относительный путь на сегменты и интегрируем в дерево
		parts := strings.Split(relPath, "/")
		entryParts := make([]models.Entry, 0)

		for _, part := range parts[:len(parts)-1] {
			entryParts = append(entryParts, models.Entry{
				IsDir:    true,
				Children: make([]models.Entry, 0),
				Name:     part,
			})
		}
		entryParts = append(entryParts, newEntry)

		m.recurseFileAdd(entryParts, 0, &result)

		return nil
	})

	if err != nil {
		return nil, err
	}

	return result, nil
}

// recurseFileAdd рекурсивно добавляет элемент в дерево, создавая промежуточные
// директории при необходимости. Аналогична методу из RepoIso.
func (m *RepoExtracted) recurseFileAdd(pathParts []models.Entry, positionInPath int, pathTree *[]models.Entry) {
	if positionInPath < len(pathParts) {
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
				m.recurseFileAdd(pathParts, positionInPath+1, &newDir.Children)
				*pathTree = append(*pathTree, newDir)
			} else { // Используем существующую
				m.recurseFileAdd(pathParts, positionInPath+1, &(*pathTree)[foundedIndex].Children)
			}
		} else { // Добавляем оконечный файл
			newFile := models.Entry{
				CreateAt: pathParts[positionInPath].CreateAt,
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
