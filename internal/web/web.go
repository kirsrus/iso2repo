package web

import (
	"context"
	"embed"
	"fmt"
	"html/template"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/gin-gonic/gin"
	"github.com/kirsrus/iso2repo/models"
)

//go:embed templates/*
var templatesFS embed.FS

var _ models.Workerses = (*Web)(nil)

// Web структура веб-сервера с использованием Gin
type Web struct {
	log *slog.Logger

	// HTTP сервер
	server *http.Server

	// Порт для прослушивания
	port int

	// Контекст для graceful shutdown
	ctx context.Context

	// Функция для инициализации маршрутов
	router *gin.Engine

	// Список репозиториев, доступных для выдачи через HTTP.
	// Ключ — имя репозитория, значение — models.Repoes.
	repos sync.Map

	// Канал получения событий обнаружения/потери репозиториев.
	changeRepos <-chan models.RepoEvent

	// HTML шаблоны
	templates *template.Template

	// Корневая директория с образами репозиториев.
	rootDir string

	// Copyright.
	copyright string

	// Версия программы.
	version string
}

// Config конфигурация веб-сервера
type Config struct {
	Log *slog.Logger

	// Порт для прослушивания
	Port int

	// Router для обработки запросов
	Router *gin.Engine

	// Канал получения событий обнаружения/потери репозиториев.
	ChangeRepos <-chan models.RepoEvent

	// Корневая директория с образами репозиториев.
	RootDir string

	// Copyright.
	Copyright string

	// Версия программы.
	Version string
}

// NewWeb конструктор веб-сервера
func NewWeb(config *Config) (*Web, error) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	if config.Log != nil {
		log = config.Log
	}

	// Установим порт по умолчанию, если не указан
	port := config.Port
	if port == 0 {
		port = 4309
	}

	router := gin.Default()
	if config.Router != nil {
		router = config.Router
	}

	changeRepos := make(<-chan models.RepoEvent)
	if config.ChangeRepos != nil {
		changeRepos = config.ChangeRepos
	}

	// Загружаем шаблоны из встроенной файловой системы
	tmpl, err := template.ParseFS(templatesFS, "templates/*")
	if err != nil {
		return nil, errors.Wrap(err, "не удалось загрузить шаблоны")
	}

	// Устанавливаем шаблоны в Gin
	router.SetHTMLTemplate(tmpl)

	m := &Web{
		log:         log.With(slog.String("module", "web")),
		port:        port,
		router:      router,
		changeRepos: changeRepos,
		templates:   tmpl,
		rootDir:     config.RootDir,
		copyright:   config.Copyright,
		version:     config.Version,
	}

	// Регистрируем обработчики HTTP запросов
	m.registerRoutes()

	return m, nil
}

// Run запускает веб-сервер. Блокирующий.
func (m *Web) Run(ctx context.Context) error {
	m.log.Debug("запуск веб-сервера")

	// Создаем HTTP сервер с указанным портом и маршрутизатором
	m.server = &http.Server{
		Addr:    fmt.Sprintf(":%d", m.port),
		Handler: m.router,
	}

	// Канал для получения ошибок сервера
	errChan := make(chan error, 1)

	// Запускаем сервер в отдельной горутине
	go func() {
		m.log.Info(fmt.Sprintf("сервер стартовал на порту :%d", m.port))
		if err := m.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errChan <- errors.Wrap(err, fmt.Sprintf("ошибка запуска веб-сервера на порту :%d", m.port))
		}
	}()

	// Запускаем горутину для прослушивания событий репозиториев
	go func() {
		for {
			select {
			case repoEvent, ok := <-m.changeRepos:
				if !ok {
					// Канал закрыт — завершаем горутину
					return
				}

				switch repoEvent.EventType {
				case models.RepoFound:
					m.repos.Store(repoEvent.Repo.Metadata().Name, repoEvent.Repo)
					m.log.Debug("репозиторий добавлен в веб-сервер", slog.String("repo", repoEvent.Repo.Metadata().Name))

				case models.RepoLost:
					m.repos.Delete(repoEvent.Repo.Metadata().Name)
					m.log.Debug("репозиторий удалён из веб-сервера", slog.String("repo", repoEvent.Repo.Metadata().Name))
				}

			case <-ctx.Done():
				return
			}
		}
	}()

	// Ожидание завершения или ошибки
	for {
		select {
		case err := <-errChan:
			// Если ошибка связана с отменой контекста — выходим штатно
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				m.log.Debug("веб-сервер остановлен по сигналу")

				return nil
			}

			m.log.Warn(fmt.Sprintf("ошибка веб-сервера: %s", err.Error()))
			return err

		case <-ctx.Done():
			m.log.Debug("веб-сервер остановлен по сигналу")

			// Graceful shutdown
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			if err := m.server.Shutdown(shutdownCtx); err != nil {
				m.log.Warn(fmt.Sprintf("ошибка graceful shutdown: %s", err.Error()))
			}

			return nil
		}
	}
}
