//go:build windows

/*
Определение программы-службы для операционной системы Windows.
*/
package cmd

import (
	"github.com/kardianos/service"
)

// program реализует интерфейс service.Interface для запуска
// основной логики iso2repo в качестве службы Windows.
type program struct{}

// Start вызывается службой при запуске. Запускаем основную логику в горутине,
// чтобы не блокировать диспетчер служб Windows.
func (p *program) Start(s service.Service) error {
	go rootRun(rootCmd, nil)
	return nil
}

// Stop вызывается службой при остановке.
func (p *program) Stop(s service.Service) error {
	return nil
}

// newServiceConfig возвращает конфигурацию службы iso2repo.
func newServiceConfig() *service.Config {
	return &service.Config{
		Name:        "iso2repo",
		DisplayName: "iso2repo",
		Description: "HTTP-транслятор локальных APT-репозиториев",
	}
}

// newService создаёт экземпляр службы iso2repo.
func newService() (service.Service, error) {
	return service.New(&program{}, newServiceConfig())
}
