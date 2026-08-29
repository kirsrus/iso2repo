//go:build !windows

/*
Запуск основной логики для операционных систем, отличных от Windows
(Linux и др.). На этих платформах программа запускается напрямую.
*/
package cmd

import "github.com/spf13/cobra"

// runAsService возвращает false, так как на Linux нет режима службы Windows.
func runAsService() bool {
	return false
}

// runService не используется на Linux.
func runService(cmd *cobra.Command, args []string) {}
