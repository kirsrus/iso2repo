package sevenz

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/thoas/go-funk"
)

func recurseFileAdd(pathParts []File, positionInPath int, pathTree *[]File) {
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
				newDir := File{
					IsDir:    true,
					Children: make([]File, 0),
					Name:     pathParts[positionInPath].Name,
				}
				recurseFileAdd(pathParts, positionInPath+1, &newDir.Children)
				*pathTree = append(*pathTree, newDir)
			} else { // Используем существующую
				recurseFileAdd(pathParts, positionInPath+1, &(*pathTree)[foundedIndex].Children)
			}
		} else { // Добаваляем оконечный файл
			newFile := File{
				CreateAt: time.Time{},
				IsDir:    false,
				Size:     pathParts[positionInPath].Size,
				FilePath: pathParts[positionInPath].FilePath,
				Children: make([]File, 0),
				Name:     pathParts[positionInPath].Name,
			}
			*pathTree = append(*pathTree, newFile)
		}
	}
}

func FindISOInDir(rootDir string) ([]string, error) {
	baseNames := make([]string, 0)

	if _, err := os.Stat(rootDir); os.IsNotExist(err) {
		return nil, errors.Errorf("не найден путь '%s'", rootDir)
	}

	result := make([]string, 0)
	err := filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {

			// ISO
			if filepath.Ext(info.Name()) == ".iso" && !funk.ContainsString(baseNames, strings.ToLower(info.Name())) {
				baseNames = append(baseNames, strings.ToLower(info.Name()))
				result = append(result, path)
			}

			// TAR
			if filepath.Ext(info.Name()) == ".tar" && !funk.ContainsString(baseNames, strings.ToLower(info.Name())) {
				baseNames = append(baseNames, strings.ToLower(info.Name()))
				result = append(result, path)
			}
		}
		return nil
	})
	if err != nil {
		return nil, errors.Annotate(err, rootDir)
	}
	return result, nil
}

// Find7Z ищет запускной файл 7z в системе
func Find7Z() (string, error) {

	// Алгоритм поиска:
	// - сначала ищем по всем путям в PATH
	// - далее ищем по стандартным установочным путям в обоих "Program Files" - x64 и x86

	// Поиск по PATH

	separator := ";"
	sevenZBinName := "7z.exe"
	if runtime.GOOS == "linux" {
		separator = ":"
		sevenZBinName = "7z"
	}

	paths := os.Getenv("PATH")
	for _, path := range strings.Split(paths, separator) {
		path = strings.TrimSpace(path)

		findPath := filepath.Join(path, sevenZBinName)

		if _, err := os.Stat(findPath); err == nil {
			return findPath, nil
		}
	}

	// Поиск в "Program Files"

	path := os.Getenv("ProgramFiles")
	finsPath := filepath.Join(path, sevenZFolderName, sevenZBinName)

	if _, err := os.Stat(finsPath); err == nil {
		return finsPath, nil
	}

	path = os.Getenv("ProgramFiles(x86)")
	if path != "" {
		finsPath := filepath.Join(path, sevenZFolderName, sevenZBinName)

		if _, err := os.Stat(finsPath); err == nil {
			return finsPath, nil
		}
	}

	return "", errors.New("программа 7z не найдена на компьютере")
}
