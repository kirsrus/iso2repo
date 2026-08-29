//go:build linux

/*
Реализация удаления сервиса для операционной системы Linux.
*/
package cmd

import (
	"fmt"
	"os"
)

const uninstallServiceFilePath = "/etc/systemd/system/iso2repo.service"

// uninstallService удаляет systemd-сервис iso2repo на Linux.
func uninstallService() error {
	// Проверка запуска от имени root.
	if !isRoot() {
		fmt.Println("Для удаления сервиса необходимо запустить программу от имени root.")
		fmt.Println("Выполните команду: sudo iso2repo uninstall")
		return nil
	}

	// Если файла сервиса нет, просто сообщаем об этом.
	if _, err := os.Stat(uninstallServiceFilePath); os.IsNotExist(err) {
		fmt.Println("Сервис iso2repo не установлен.")
		return nil
	}

	// Останавливаем и отключаем службу.
	if err := runSystemctl("stop", "iso2repo.service"); err != nil {
		return err
	}
	if err := runSystemctl("disable", "iso2repo.service"); err != nil {
		return err
	}
	if err := runSystemctl("daemon-reload"); err != nil {
		return err
	}

	// Удаляем файл юнита сервиса.
	if err := os.Remove(uninstallServiceFilePath); err != nil {
		return fmt.Errorf("не удалось удалить файл сервиса %s: %w", uninstallServiceFilePath, err)
	}

	fmt.Printf("Файл сервиса удалён: %s\n", uninstallServiceFilePath)

	return nil
}
