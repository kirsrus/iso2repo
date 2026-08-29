//go:build windows

/*
Реализация удаления службы для операционной системы Windows.
Используется библиотека github.com/kardianos/service.
*/
package cmd

import (
	"fmt"
	"os"

	"github.com/kardianos/service"
)

// uninstallService удаляет службу iso2repo на Windows.
func uninstallService() error {
	// Если программа запущена не от имени администратора, запрашиваем
	// повышение прав через UAC и завершаем текущий процесс.
	if ok, err := isAdmin(); err != nil {
		return fmt.Errorf("не удалось проверить права администратора: %w", err)
	} else if !ok {
		fmt.Println("Требуются права администратора. Запрашиваю повышение прав...")
		if err := elevateToAdmin(); err != nil {
			return fmt.Errorf("не удалось запросить повышение прав: %w", err)
		}
		os.Exit(0) // после вызова UAC повышенная копия продолжит работу
	}

	s, err := newService()
	if err != nil {
		return fmt.Errorf("не удалось создать службу: %w", err)
	}

	// Проверяем наличие службы. Если её нет, просто сообщаем об этом.
	status, errStatus := s.Status()
	if errStatus != nil || status == service.StatusUnknown {
		fmt.Println("Служба iso2repo не установлена.")
		return nil
	}

	// Останавливаем службу, если она запущена.
	if status != service.StatusStopped {
		fmt.Println("Останавливаю службу iso2repo...")
		if err := s.Stop(); err != nil {
			return fmt.Errorf("не удалось остановить службу: %w", err)
		}
	}

	// Удаляем службу.
	if err := s.Uninstall(); err != nil {
		return fmt.Errorf("не удалось удалить службу: %w", err)
	}
	fmt.Println("Служба iso2repo удалена.")

	return nil
}
