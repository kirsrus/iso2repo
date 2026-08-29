//go:build windows

/*
Заглушка удаления сервиса для операционной системы Windows.
Реализация будет добавлена позже.
*/
package cmd

import "fmt"

// uninstallService удаляет сервис iso2repo на Windows.
// В настоящее время функционал недоступен.
func uninstallService() error {
	fmt.Println("Удаление сервиса для Windows пока не реализовано.")
	fmt.Println("Данная функциональность будет доступна в следующих версиях программы.")
	return nil
}
