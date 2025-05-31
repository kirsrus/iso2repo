package cmd

// Template описывает шаблоны
type Template struct {

	// Шаблон главной страницы
	Index string

	// Шаблон дерева файлов
	Tree string

	// Шаблон описания source.list
	Sources string
}

// TemplateIndex данные для передачи в шаблон Index
type TemplateIndex struct {
	Title string

	// Версия программы
	Version string

	// Время коммита
	Date string

	Copyright string

	// Колличество обнаруженных ISO-файлов
	ISOPathLen int

	// Путь к ISO-файлам
	ISOPath []string

	// Список IP-адресов
	IPList []string
}

// TemplateSources данные для передачи в шаблон Sources
type TemplateSources struct {
	Title string

	// Версия программы
	Version string

	// Время коммита
	Date string

	Copyright string

	// Стрки поключения репозиториев
	DebPath []string

	// Список IP-адресов
	IPList []string
}

// TemplatesFiles описывает вывод файловой системы ISO-образов
type TemplatesFiles struct {
	Title string

	// Версия программы
	Version string

	// Время коммита
	Date string

	Copyright string

	// URL возврата обратно (..)
	BackwardURL File

	FilesOrDir []File
}

// File описывает файл в структуре ISO
type File struct {
	IsDir bool
	Name  string
	URL   string
	Size  string
}

// Tree данные для передачи в шаблон Tree
type Tree struct {

	// Версия программы
	Version string

	ISO string

	Action string
}
