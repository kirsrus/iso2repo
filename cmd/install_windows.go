//go:build windows

/*
Заглушка установки сервиса для операционной системы Windows.
Реализация будет добавлена позже.
*/
package cmd

import "fmt"

// installService устанавливает iso2repo в качестве сервиса на Windows.
// В настоящее время функционал недоступен.
func installService() error {
	fmt.Println("Установка сервиса для Windows пока не реализована.")
	fmt.Println("Данная функциональность будет доступна в следующих версиях программы.")
	return nil
}
