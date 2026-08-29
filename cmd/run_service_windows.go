//go:build windows

/*
Запуск основной логики в режиме службы Windows.
*/
package cmd

import (
	"fmt"

	"github.com/kardianos/service"
	"github.com/spf13/cobra"
)

// runAsService возвращает true, если программа запущена как служба Windows
// (не в интерактивной сессии).
func runAsService() bool {
	return !service.Interactive()
}

// runService запускает программу через службу Windows и блокирует выполнение
// до завершения работы службы.
func runService(cmd *cobra.Command, args []string) {
	s, err := newService()
	if err != nil {
		fmt.Println("Ошибка создания службы:", err)
		return
	}
	if err := s.Run(); err != nil {
		fmt.Println("Ошибка запуска службы:", err)
	}
}
