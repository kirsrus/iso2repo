package sevenz

import "time"

// File олицетворяет описание файла в ISO-образе в виде дерева с потомками в Children. Корневая папка создаётся
// через функцию NewRoot
type File struct {
	CreateAt time.Time
	IsRoot   bool // Является ли это корневая директорию
	IsDir    bool
	Size     int
	FilePath string

	// Для построения дерева

	Children []File
	Name     string
}

func NewRoot() File {
	return File{
		CreateAt: time.Time{},
		IsRoot:   true,
		IsDir:    true,
		Size:     0,
		FilePath: "",
		Name:     "",
		Children: make([]File, 0),
	}
}
