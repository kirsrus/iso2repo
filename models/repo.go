package models

import (
	"context"
	"io"
	"time"
)

// RepoType описывает тип репозитория.
type RepoType int

const (
	// Стандартный ISO образ с репозиторием.
	RepoISO RepoType = iota
	// Распакованный репозиторий в виде директории (с расширением .iso).
	RepoExtracted
	// Пользовательский набор файлов для динамически создаваемого репозитория.
	RepoCustom
)

// Repo описывает репозиторий.
type Repo struct {
	// Имя репозитория (отображает имя файла или директории с репозиторием).
	Name string
	// Асболютный путь к файлу или папке репозитория.
	Path string
	// Тип репозитория.
	Type RepoType
}

// RepoEventType описывает тип события, закреплённого за репозиторием (обнаружение, потеря).
type RepoEventType int

const (
	// Реопзиторий обраружен.
	RepoFound RepoEventType = iota

	// Репозиторий потерян.
	RepoLost
)

// RepoEvent описание события обнаружения репозитория.
type RepoEvent struct {
	Repo      Repoes
	EventType RepoEventType
}

// Repoes определяет структуру работы с репозиторями.
type Repoes interface {

	// Metadata возвращает метаданные репозитория.
	Metadata() Repo

	// IsRepo возвращает true, если источник является репозиторием.
	IsRepo() bool

	// RepoString возвращает строковое представление данных в репозитории для клиентов.
	// Например:
	//
	//   deb [arch=amd64] http://repo.loc:4309/repo/1.7_loader.iso 1.7_x86-64 contrib main non-free
	RepoString() string

	// List возвращает список записей по указанному пути внутри образа.
	// Путь должен быть относительным корня образа, без ведущего "/".
	// Для корневого каталога передаётся пустая строка.
	List(ctx context.Context, path string) ([]Entry, error)

	// Open открывает файл для потокового чтения.
	// Возвращает io.ReadCloser; вызывающий код обязан закрыть его.
	// Реализация должна поддерживать чтение блоками через stdout 7z
	// для больших файлов, не загружая всё содержимое в память.
	Open(ctx context.Context, path string) (io.ReadCloser, error)
}

// Entry описывает файл или директорию внутри ISO-образа.
type Entry struct {
	CreateAt time.Time
	Name     string
	FilePath string
	IsDir    bool
	Size     int64

	Children []Entry
}
