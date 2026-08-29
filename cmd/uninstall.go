/*
Copyright © 2026 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// uninstallCmd представляет команду uninstall
var uninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Удаление сервиса iso2repo",
	Long: `Удаляет системный сервис iso2repo.
На Linux удаляет systemd-юнит iso2repo.service.`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := uninstallService(); err != nil {
			fmt.Println("Ошибка удаления сервиса:", err)
		}
	},
}

func init() {
	rootCmd.AddCommand(uninstallCmd)

	// Здесь вы можете определить флаги и параметры конфигурации.

	// Cobra поддерживает Persistent Flags, которые будут работать для этой команды
	// и всех подкоманд, например:
	// uninstallCmd.PersistentFlags().String("foo", "", "Справка для foo")

	// Cobra поддерживает локальные флаги, которые будут работать только при прямом
	// вызове этой команды, например:
	// uninstallCmd.Flags().BoolP("toggle", "t", false, "Справка для toggle")
}
