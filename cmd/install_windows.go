//go:build windows

/*
Реализация установки сервиса для операционной системы Windows.
Используется библиотека github.com/kardianos/service.
*/
package cmd

import (
	"fmt"
	"os"
	"strings"
	"unsafe"

	"github.com/kardianos/service"
	"golang.org/x/sys/windows"
)

// installService устанавливает iso2repo в качестве службы Windows.
func installService() error {
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

	// Если служба уже существует, останавливаем её и удаляем,
	// чтобы заменить текущей реализацией.
	if status, errStatus := s.Status(); errStatus == nil && status != service.StatusUnknown {
		fmt.Println("Служба iso2repo уже установлена. Выполняю замену.")
		_ = s.Stop()
		if err := s.Uninstall(); err != nil {
			return fmt.Errorf("не удалось удалить существующую службу: %w", err)
		}
	}

	// Устанавливаем новую службу. Путь к исполняемому файлу iso2repo.exe
	// определяется автоматически из запущенного процесса.
	if err := s.Install(); err != nil {
		return fmt.Errorf("не удалось установить службу: %w", err)
	}
	fmt.Println("Служба iso2repo установлена.")

	// Запускаем службу сразу после установки.
	if err := s.Start(); err != nil {
		return fmt.Errorf("не удалось запустить службу: %w", err)
	}
	fmt.Println("Служба iso2repo запущена.")

	return nil
}

// isAdmin определяет, запущен ли процесс с повышенными правами
// (от имени администратора).
func isAdmin() (bool, error) {
	var token windows.Token
	if err := windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_QUERY, &token); err != nil {
		return false, err
	}
	defer token.Close()

	return token.IsElevated(), nil
}

// elevateToAdmin перезапускает программу с повышением прав через UAC
// с помощью ShellExecuteW и операции "runas".
func elevateToAdmin() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}

	params := strings.Join(os.Args[1:], " ")

	verbPtr, _ := windows.UTF16PtrFromString("runas")
	filePtr, _ := windows.UTF16PtrFromString(exe)
	paramPtr, _ := windows.UTF16PtrFromString(params)

	shell32 := windows.NewLazySystemDLL("shell32.dll")
	procShellExecuteW := shell32.NewProc("ShellExecuteW")

	r1, _, err := procShellExecuteW.Call(
		0,                                 // hwnd
		uintptr(unsafe.Pointer(verbPtr)),  // операция
		uintptr(unsafe.Pointer(filePtr)),  // файл
		uintptr(unsafe.Pointer(paramPtr)), // параметры
		0,                                 // каталог
		1,                                 // SW_SHOWNORMAL
	)
	if r1 <= 32 {
		return fmt.Errorf("ShellExecuteW завершился с кодом %d: %w", r1, err)
	}
	return nil
}
