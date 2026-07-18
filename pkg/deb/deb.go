package deb

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/blakesmith/ar"
	"github.com/klauspost/compress/zstd"
	"github.com/ulikunitz/xz"
)

// PackageMeta — структура для хранения метаинформации из control-файла .deb пакета.
type PackageMeta struct {
	Package       string            // Название пакета
	Version       string            // Версия пакета
	Architecture  string            // Архитектура (amd64, arm64 и т.д.)
	Maintainer    string            // Контактное лицо или команда сопровождающих
	InstalledSize string            // Установленный размер пакета в килобайтах
	Depends       string            // Зависимости, необходимые для установки
	PreDepends    string            // Предварительные зависимости (устанавливаются до пакета)
	Recommends    string            // Рекомендуемые дополнительные пакеты
	Suggests      string            // Предлагаемые дополнительные пакеты
	Conflicts     string            // Пакеты, конфликтующие с данным пакетом
	Replaces      string            // Пакеты, которые заменяет данный пакет
	Provides      string            // Виртуальные пакеты, предоставляемые данным пакетом
	Section       string            // Категория пакета (например, utils, net, admin)
	Priority      string            // Приоритет установки (required, important, standard)
	Homepage      string            // URL домашней страницы проекта
	Description   string            // Описание пакета
	Extra         map[string]string // Все остальные поля, не вошедшие в стандартные
}

// ExtractMeta читает .deb файл и возвращает его метаинформацию.
// Принимает путь к файлу .deb (может содержать как Windows, так и Linux слеши).
// Возвращает структуру PackageMeta с данными из control-файла или ошибку.
func ExtractMeta(debPath string) (*PackageMeta, error) {
	// Заменяем все Windows слеши на Linux слеши для совместимости
	debPath = strings.ReplaceAll(debPath, "\\", "/")

	f, err := os.Open(debPath)
	if err != nil {
		return nil, fmt.Errorf("не удалось открыть .deb файл: %w", err)
	}
	defer f.Close()

	arR := ar.NewReader(f)
	for {
		hdr, err := arR.Next()
		if err == io.EOF {
			return nil, fmt.Errorf("архив control не найден в .deb файле")
		}
		if err != nil {
			return nil, fmt.Errorf("ошибка чтения архива .deb: %w", err)
		}

		name := strings.TrimSpace(hdr.Name)
		name = strings.Trim(name, "/")
		if !strings.HasPrefix(name, "control.tar") {
			continue
		}

		var r io.Reader
		switch {
		case strings.HasSuffix(name, ".gz"):
			gr, err := gzip.NewReader(arR)
			if err != nil {
				return nil, fmt.Errorf("ошибка инициализации gzip: %w", err)
			}
			defer gr.Close()
			r = gr
		case strings.HasSuffix(name, ".xz"):
			xr, err := xz.NewReader(arR)
			if err != nil {
				return nil, fmt.Errorf("ошибка инициализации xz: %w", err)
			}
			r = xr
		case strings.HasSuffix(name, ".zst"):
			zr, err := zstd.NewReader(arR)
			if err != nil {
				return nil, fmt.Errorf("ошибка инициализации zstd: %w", err)
			}
			defer zr.Close()
			r = zr
		default:
			r = arR
		}

		return parseControlTar(r)
	}
}

// parseControlTar парсит архив control.tar.* и извлекает control-файл.
// Принимает io.Reader для чтения архива control.
// Возвращает структуру PackageMeta или ошибку.
func parseControlTar(r io.Reader) (*PackageMeta, error) {
	tr := tar.NewReader(r)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil, fmt.Errorf("control файл не найден в архиве control.tar")
		}
		if err != nil {
			return nil, fmt.Errorf("ошибка чтения tar архива: %w", err)
		}
		// Пропускаем все файлы, кроме control
		if strings.TrimPrefix(hdr.Name, "./") != "control" {
			continue
		}

		fields, err := parseControl(tr)
		if err != nil {
			return nil, err
		}
		return fieldsToMeta(fields), nil
	}
}

// parseControl парсит control-файл и извлекает ключевые-значения.
// Принимает io.Reader для чтения control-файла.
// Возвращает map[string]string с парами ключ-значение или ошибку.
func parseControl(r io.Reader) (map[string]string, error) {
	fields := make(map[string]string)
	sc := bufio.NewScanner(r)
	var key string
	var val strings.Builder

	// Вспомогательная функция для сохранения текущего поля
	flush := func() {
		if key != "" {
			fields[key] = strings.TrimSpace(val.String())
		}
	}

	for sc.Scan() {
		line := sc.Text()
		// Пустая строка означает конец записей
		if line == "" {
			flush()
			break
		}
		// Обработка продолжения строки (строки, начинающиеся с пробела или табуляции)
		if line[0] == ' ' || line[0] == '\t' {
			val.WriteByte('\n')
			val.WriteString(strings.TrimSpace(line))
			continue
		}
		// Сохраняем предыдущее поле перед началом нового
		flush()
		// Ищем разделитель между ключом и значением
		i := strings.IndexByte(line, ':')
		if i < 0 {
			key = ""
			continue
		}
		key = strings.TrimSpace(line[:i])
		val.Reset()
		val.WriteString(strings.TrimSpace(line[i+1:]))
	}
	flush()
	return fields, sc.Err()
}

// knownKeys — множество известных ключей control-файла.
// Используется для определения стандартных полей при парсинге.
var knownKeys = map[string]bool{
	"Package":        true,
	"Version":        true,
	"Architecture":   true,
	"Maintainer":     true,
	"Installed-Size": true,
	"Depends":        true,
	"Pre-Depends":    true,
	"Recommends":     true,
	"Suggests":       true,
	"Conflicts":      true,
	"Replaces":       true,
	"Provides":       true,
	"Section":        true,
	"Priority":       true,
	"Homepage":       true,
	"Description":    true,
}

// fieldsToMeta преобразует map[string]string в структуру PackageMeta.
// Принимает map с парами ключ-значение из control-файла.
// Возвращает заполненную структуру PackageMeta.
func fieldsToMeta(f map[string]string) *PackageMeta {
	m := &PackageMeta{
		Package:       f["Package"],
		Version:       f["Version"],
		Architecture:  f["Architecture"],
		Maintainer:    f["Maintainer"],
		InstalledSize: f["Installed-Size"],
		Depends:       f["Depends"],
		PreDepends:    f["Pre-Depends"],
		Recommends:    f["Recommends"],
		Suggests:      f["Suggests"],
		Conflicts:     f["Conflicts"],
		Replaces:      f["Replaces"],
		Provides:      f["Provides"],
		Section:       f["Section"],
		Priority:      f["Priority"],
		Homepage:      f["Homepage"],
		Description:   f["Description"],
		Extra:         make(map[string]string),
	}
	// Все неизвестные поля сохраняем в Extra
	for k, v := range f {
		if !knownKeys[k] {
			m.Extra[k] = v
		}
	}
	return m
}
