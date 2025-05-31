package main

import (
	_ "embed"

	"github.com/kirsrus/iso2repo/cmd"
)

// Версия программы. Берётся из GIT тэга версии при компиляции через -ldflags
var version = "0.0.0"

// Номер текущего коммита. Берётся из GIT тэга коммита при компиляции через -ldflags.
var gitCommit = "00000000000000000"

// Дата и время последнего коммита. Берётся из GIT времени коммита при компиляции через -ldflags.
var gitDate = "0000.00.00 00:00:00"

//go:embed templates/index.html
var indexTpl string

//go:embed templates/tree.html
var treeTpl string

//go:embed templates/sources.html
var sourcesTpl string

func main() {
	cmd.Execute(version, gitCommit, gitDate, cmd.Template{
		Index:   indexTpl,
		Tree:    treeTpl,
		Sources: sourcesTpl,
	})
}
