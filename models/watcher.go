package models

// File описание обнаруженных файлов.
type File struct {
	// Имя файла без пути к нему.
	Name string

	// Абслоютный путь к файлу.
	Path string
}

// FileEventType описывает тип события, закреплённого за файлом (обнаружение, потеря).
type FileEventType int

const (
	// Файл обнаружен.
	FileFound FileEventType = iota

	// Файл потерян.
	FileLost
)

// FileEvent описание события обнаружения файла.
type FileEvent struct {
	File      File
	EventType FileEventType
}
