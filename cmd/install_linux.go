//go:build linux

/*
Реализация установки сервиса для операционной системы Linux.
*/
package cmd

import (
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
)

//go:embed service/iso2repo.service
var serviceTemplate string

const serviceFilePath = "/etc/systemd/system/iso2repo.service"

// installService устанавливает iso2repo в качестве systemd-сервиса на Linux.
func installService() error {
	// Проверка запуска от имени root.
	if !isRoot() {
		fmt.Println("Для установки сервиса необходимо запустить программу от имени root.")
		fmt.Println("Выполните команду: sudo iso2repo install")
		return nil
	}

	// Определяем путь к исполняемому файлу программы.
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("не удалось определить путь к исполняемому файлу: %w", err)
	}
	execPath, err = filepath.Abs(execPath)
	if err != nil {
		return fmt.Errorf("не удалось получить абсолютный путь к исполняемому файлу: %w", err)
	}

	// Если сервис уже установлен, останавливаем его и удаляем,
	// чтобы заменить текущей реализацией.
	if _, err := os.Stat(serviceFilePath); err == nil {
		fmt.Println("Сервис iso2repo уже установлен. Выполняю замену.")
		runSystemctl("stop", "iso2repo")
		runSystemctl("disable", "iso2repo")
		if err := os.Remove(serviceFilePath); err != nil {
			return fmt.Errorf("не удалось удалить существующий файл сервиса %s: %w", serviceFilePath, err)
		}
	}

	// Подставляем путь к программе в шаблон сервиса.
	content := strings.ReplaceAll(serviceTemplate, "{{.ExecPath}}", execPath)

	// Записываем файл юнита сервиса.
	if err := os.WriteFile(serviceFilePath, []byte(content), 0644); err != nil {
		return fmt.Errorf("не удалось записать файл сервиса %s: %w", serviceFilePath, err)
	}
	fmt.Printf("Файл сервиса создан: %s\n", serviceFilePath)
	fmt.Printf("Исполняемый файл: %s\n", execPath)

	// Активируем сервис.
	if err := runSystemctl("daemon-reload"); err != nil {
		return err
	}
	if err := runSystemctl("enable", "iso2repo"); err != nil {
		return err
	}
	if err := runSystemctl("start", "iso2repo"); err != nil {
		return err
	}

	fmt.Println("Сервис iso2repo установлен и запущен.")
	return nil
}

// runSystemctl выполняет команду systemctl и возвращает ошибку в случае неудачи.
func runSystemctl(args ...string) error {
	cmd := exec.Command("systemctl", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ошибка выполнения systemctl %s: %v\n%s", strings.Join(args, " "), err, output)
	}
	return nil
}

// isRoot проверяет, что процесс запущен от имени пользователя root.
func isRoot() bool {
	u, err := user.Current()
	if err != nil {
		return false
	}
	return u.Uid == "0" || u.Username == "root"
}
