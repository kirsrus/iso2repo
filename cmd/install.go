/*
Copyright © 2026 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// installCmd представляет команду install
var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Установка iso2repo в качестве системного сервиса",
	Long: `Устанавливает iso2repo в качестве системного сервиса.
На Linux создаёт и регистрирует systemd-юнит iso2repo.service,
запускающий программу под root.`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := installService(); err != nil {
			fmt.Println("Ошибка установки сервиса:", err)
		}
	},
}

func init() {
	rootCmd.AddCommand(installCmd)

	// Здесь вы можете определить флаги и параметры конфигурации.

	// Cobra поддерживает Persistent Flags, которые будут работать для этой команды
	// и всех подкоманд, например:
	// installCmd.PersistentFlags().String("foo", "", "Справка для foo")

	// Cobra поддерживает локальные флаги, которые будут работать только при прямом
	// вызове этой команды, например:
	// installCmd.Flags().BoolP("toggle", "t", false, "Справка для toggle")
}
