package sevenz

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cast"
	"github.com/thoas/go-funk"
)

var (
	ErrIsNotRepo = errors.New("is not repo")

	warn7zVersionNotFoundKey     = false
	warn7zVersionNotSupportedKey = false
)

const (
	sevenZFolderName = "7-Zip"
)

type SevenZ struct {
	log *logrus.Entry

	// Путь к обрабатываему ISO-файлу
	ISOPath string

	// Версия программы
	Version string

	// Путь к бинарнику 7z
	sevenZPath string

	// Дерево файлов в образе ISO
	filesTree []File

	// Стока для подключения репозитория извне
	sourceString string

	// Префикс для путей к файлам внутри архива. Нужен для отличия путей в ISO файлах (вида boot\grub\i386-efi\fdt.lst),
	// от путей TAR файлов (вида .\dists\1.7_x86-64\main, т.е. с префиксом `.\`)
	pathPrefix string
}

// NewSevenZ читает файл ISO-репозитория.
// Если ISO-образ не является репозиторием, возвращается ошибка ErrIsNotRepo
func NewSevenZ(ISOPath string, log *logrus.Logger) (*SevenZ, error) {
	var err error
	if log == nil {
		log = logrus.New()
		log.Out = io.Discard
	}

	if _, err := os.Stat(ISOPath); os.IsNotExist(err) {
		return nil, errors.Annotate(err, ISOPath)
	}

	m := SevenZ{
		log:     log.WithField("scope", "7z"),
		ISOPath: ISOPath,
	}

	m.sevenZPath, err = m.find7ZBin()
	if err != nil {
		return nil, errors.Trace(err)
	}

	m.Version, err = m.read7ZVersion()
	if err != nil {
		return nil, errors.Trace(err)
	} else if m.Version == "0.0.0" {
		if !warn7zVersionNotFoundKey {
			m.log.Warn("не удалось определить номер версии 7z (результат работы программы не гарантирован)")
			warn7zVersionNotFoundKey = true
		}
	} else {
		versionStr := strings.Split(m.Version, ".")
		version := make([]int, 0)
		funk.ForEach(versionStr, func(v string) {
			version = append(version, cast.ToInt(v))
		})

		if version[0] < 16 || version[0] > 22 {
			if !warn7zVersionNotSupportedKey {
				m.log.Warnf("версия 7z %s не проверялась (результат работы программы не гарантирован)", m.Version)
				warn7zVersionNotSupportedKey = true
			}
		}
	}

	m.filesTree, err = m.readFilesFromISO(m.ISOPath)
	if err != nil {
		return nil, errors.Annotate(err, m.ISOPath)
	}

	// Формируем строку для подключения. По этой строке понимаем, что это диск с репозиторием или нет
	m.sourceString, err = m.createSourceString()
	if err != nil {
		return nil, errors.Trace(err)
	}

	return &m, nil
}

func (m *SevenZ) readFilesFromISO(ISOPath string) ([]File, error) {
	output, err := m.exec7zOnce([]string{"l", ISOPath})
	if err != nil {
		return nil, errors.Trace(err)
	}

	result := make([]File, 0)

	reLine := regexp.MustCompile(`^(\d+-\d+-\d+)\s+(\d+:\d+:\d+)\s+(.|D)\.\.\.\.\s+(\d*)\s+(\d*)\s+([^ ]+)$`)

	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)

		match := reLine.FindStringSubmatch(line)
		if len(match) != 0 {

			// Если это TAR архив (определяем по наличию директории '.',
			// то устанавливаем префикс доступа к файлам. Саму же директорию пропускаем
			if match[6] == "." {
				m.pathPrefix = ".\\"
				continue
			}
			match[6] = strings.TrimPrefix(match[6], m.pathPrefix)

			newFileOrDir := File{
				IsDir: match[3] == "D",
				Size:  cast.ToInt(match[4]),
				Name:  filepath.Base(match[6]),
			}

			// Нормализуем время
			testTime := match[1] + " " + match[2]
			t, err := time.Parse("2006-01-02 15:04:05", testTime)
			if err != nil {
				m.log.Warnf("не удалось распарсить время создания файла/директории '%s'", testTime)
			}
			newFileOrDir.CreateAt = t

			// Нормализуем путь и интегрируем в дерево путей

			filePathParts := make([]File, 0)
			filePath := strings.ReplaceAll(match[6], "\\", "/")
			filePath = strings.TrimPrefix(filePath, "/")
			filePath = strings.TrimSuffix(filePath, "/")

			filePathSplit := strings.Split(filePath, "/")
			if len(filePathSplit) > 1 {
				for _, pathPart := range filePathSplit[:len(filePathSplit)-1] {
					filePathParts = append(filePathParts, File{
						IsDir:    true,
						Children: make([]File, 0),
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

// Определяет версию установленного 7z. Если версию определить не удалось, возвращается
// версия "0.0.0"
func (m SevenZ) read7ZVersion() (string, error) {
	output, err := m.exec7zOnce([]string{})
	if err != nil {
		return "", errors.Trace(err)
	}

	reList := []*regexp.Regexp{
		// 7-Zip 22.01 (x64) : Copyright (c) 1999-2022 Igor Pavlov : 2022-07-15
		regexp.MustCompile(`^[0-9a-zA-z-]+\s+(\d+.\d+)`),
		// 7-Zip [64] 16.02 : Copyright (c) 1999-2016 Igor Pavlov : 2016-05-21
		regexp.MustCompile(`^[0-9a-zA-z-]+\s+\[\d+]\s+(\d+.\d+)`),
	}

	for lineIdx, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)

		if lineIdx == 0 {

			match := make([]string, 0)
			for _, re := range reList {
				match = re.FindStringSubmatch(line)
				if len(match) != 0 {
					break
				}
			}
			if len(match) == 0 {
				return "0.0.0", nil
			}

			result := make([]string, 0)
			for _, i := range strings.Split(match[1], ".") {
				result = append(result, cast.ToString(cast.ToInt(i)))
			}
			result = append(result, "0")

			return strings.Join(result, "."), nil
		}
	}

	return "0.0.0", nil
}

// ReadPath по пути path возвращает или целевой файл, или списки директорий и файлов
// на уровне, указанном в path
func (m SevenZ) ReadPath(pathDirOrFile string) (*File, []File, error) {
	pathItem := strings.TrimSpace(pathDirOrFile)
	pathItem = strings.TrimPrefix(pathItem, "/")
	pathItem = strings.TrimSuffix(pathItem, "/")

	// Определяем папку
	currentLevel := m.filesTree

	pathParts := strings.Split(pathItem, "/")
	if len(pathParts) > 1 {
		for _, pathPart := range pathParts[:len(pathParts)-1] {
			found := false
			for _, v := range currentLevel {
				if v.Name == pathPart {
					currentLevel = v.Children
					found = true
					break
				}
			}

			if !found {
				m.log.Errorf("не найден путь '%s'", pathDirOrFile)
				return nil, nil, errors.Errorf("не найден путь '%s'", pathDirOrFile)
			}
		}
	}

	// Заходим в директорию или возвращаем файл
	if pathItem != "" { // Корневая директория
		pathEndFile := path.Base(pathItem)
		found := false
		for _, v := range currentLevel {
			if v.Name == pathEndFile {
				if v.IsDir {
					currentLevel = v.Children
				} else {
					return &v, make([]File, 0), nil
				}
				found = true
				break
			}
		}

		if !found {
			m.log.Errorf("не найден путь '%s:%s'", filepath.Base(m.ISOPath), pathDirOrFile)
			return nil, nil, errors.Errorf("не найден путь '%s", pathDirOrFile)
		}
	}

	// Читаем содержимое папки
	inDirs := make([]File, 0)
	inFiles := make([]File, 0)

	for _, v := range currentLevel {
		if v.IsDir {
			inDirs = append(inDirs, v)
		} else {
			inFiles = append(inFiles, v)
		}
	}

	sort.Slice(inDirs, func(i, j int) bool { return inDirs[i].Name < inDirs[j].Name })
	sort.Slice(inFiles, func(i, j int) bool { return inFiles[i].Name < inFiles[j].Name })

	return nil, append(inDirs, inFiles...), nil
}

// readDir возвращает содержимое директории со всеми потомками по абсолютному пути
// Разделитель путей '/' без начального слэша:
//
//	"path/to/dir"
//
// Если директория не найдена, возвращается ошибка os.ErrNotExist
func (m *SevenZ) readDir(absDirPath string) ([]File, error) {
	absDirPathClean := absDirPath
	absDirPathClean = strings.ReplaceAll(absDirPathClean, "\\", "/")
	absDirPathClean = strings.TrimSpace(absDirPathClean)
	absDirPathClean = strings.TrimLeft(absDirPathClean, "/")

	absDirPathSplit := strings.Split(absDirPathClean, "/")

	currentFolder := m.filesTree
	for _, pathBit := range absDirPathSplit {
		foundDir := false
		for i := range currentFolder {
			if currentFolder[i].Name == pathBit && currentFolder[i].IsDir {
				currentFolder = currentFolder[i].Children
				foundDir = true
				break
			}

		}
		if !foundDir {
			// Путь не найден
			return nil, os.ErrNotExist
		}
	}
	return currentFolder, nil
}

// createSourceString создаёт строку подключения репозитория в sources.list. Если строку сделать не удалось,
// и возвратилась ошибка ErrIsNotRepo - значит это не диск с репозиторием.
// Так как неизвестен на данном этапе IP сервера, он указывается как 'http://0.0.0.0'
func (m *SevenZ) createSourceString() (string, error) {
	distPath := "dists"

	inDist, err := m.readDir(distPath)
	if err != nil {
		return "", ErrIsNotRepo
	}

	distributeName := ""
	for _, v := range inDist {
		if v.IsDir {
			distributeName = v.Name
		}
	}
	if distributeName == "" {
		return "", ErrIsNotRepo
	}

	findPath := path.Join(distPath, distributeName)
	inDistSub, err := m.readDir(findPath)
	if err != nil {
		return "", ErrIsNotRepo
	}

	releaseName := ""
	for _, v := range inDistSub {
		if v.Name == "Release" && !v.IsDir {
			releaseName = v.Name
			break
		}
	}
	if releaseName == "" {
		return "", ErrIsNotRepo
	}

	releaseBuff := new(bytes.Buffer)
	findPath = path.Join(distPath, distributeName, releaseName)
	err = m.ReadFile(findPath, releaseBuff)
	if err != nil {
		return "", errors.Trace(err)
	}

	// Разбираем содержимое файла Release
	findPrefix := "components:"
	components := make([]string, 0)
	for _, v := range strings.Split(releaseBuff.String(), "\n") {
		line := strings.TrimSpace(v)
		if strings.HasPrefix(strings.ToLower(line), findPrefix) {
			lineClean := strings.ReplaceAll(line, "\t", " ")
			linePart := strings.Split(lineClean[len(findPrefix):], " ")
			for _, p := range linePart {
				pClean := strings.TrimSpace(p)
				if pClean != "" && !funk.ContainsString(components, pClean) {
					// Проверяем, что такая директория существует
					findDir := path.Join(distPath, distributeName, pClean)
					_, err := m.readDir(findDir)
					if err != nil {
						continue
					}
					// Директория существует
					components = append(components, pClean)
				}
			}
			break
		}
	}
	if len(components) == 0 {
		return "", errors.Errorf("в файле '%s/%s/Release' не найден блок '%s'", distPath, distributeName, findPrefix)
	}

	// Подготовка результата
	sort.Strings(components)
	result := fmt.Sprintf("deb http://0.0.0.0/repo/%s %s %s", filepath.Base(m.ISOPath), distributeName, strings.Join(components, " "))

	return result, nil
}

// GetRepoString возвращает строку подключения в sources.list
// Так как неизвестен на данном этапе IP сервера, он указывается как 'http://0.0.0.0'
func (m SevenZ) GetRepoString() string {
	return m.sourceString
}
