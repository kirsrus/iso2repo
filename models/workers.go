package models

import "context"

// Wokrerses описывает интерфейсы воркеров.
type Workerses interface {
	// Запуск воркера. Остановка производится через контекст ctx. Запуск блокирующий.
	// В случае ошибки возвращает error.
	Run(ctx context.Context) error
}
