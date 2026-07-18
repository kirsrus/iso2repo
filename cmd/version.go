/*
Copyright © 2026 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// versionCmd represents the version command
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Версия программы",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		v := strings.ReplaceAll(version, "v", "")
		fmt.Println(v)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
