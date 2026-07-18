package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kirsrus/iso2repo/internal/repo"
	"github.com/kirsrus/iso2repo/internal/watcher"
	"github.com/kirsrus/iso2repo/internal/web"
	"github.com/kirsrus/iso2repo/models"
	"github.com/kirsrus/iso2repo/pkg/logging"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
)

// Версия программы. Будет переопределено при сборке через -ldflags
var version string = "v0.0.0"

const copyright = "Стерликов Кирилл @2023-2026"

var rootCmd = &cobra.Command{
	Use:   "iso2repo",
	Short: "HTTP-транслятор локальных APT-репозиториев",
	Long: `Программа решает задачу подключения к локально расположенным APT-репозиториям.
Репозитории заранее скачиваются и упаковываются в ISO-образы. Или создаётся папка с расширением .iso и туда
помещаются файлы с расширением .deb. iso2repo отслеживает директорию с образами, распознаёт в них структуру
APT-репозитория и предоставляет к ним доступ по HTTP в локальной сети.
`,
	SilenceErrors: true, // Отключает вывод описания ошибок
	SilenceUsage:  true, // Отключает вывод текста "описание использования" при ошибке
	Run:           rootRun,
}

func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().String(FlagLevel, "info", "уровень логирования (debug|info|warn|error)")
	rootCmd.PersistentFlags().String(FlagRootDir, "", "корневая директория со образами репозиториев")
	rootCmd.PersistentFlags().Duration(FlagPollInterval, 60*time.Second, "интервал опроса директории")
	rootCmd.PersistentFlags().Int(FlagPort, 4309, "порт WEB-интерфейса")
}

func rootRun(cmd *cobra.Command, _ []string) {
	var err error
	levelFlag, _ := cmd.Flags().GetString(FlagLevel)

	log := logging.NewLoggingWithStringLevel(levelFlag, 1)
	log.Info(fmt.Sprintf("программа стартовала; версия %s", version))

	rootDir, _ := cmd.Flags().GetString(FlagRootDir)
	if rootDir == "" {
		rootDir = filepath.Dir(os.Args[0])
		log.Info(fmt.Sprintf("автоматически определена корневая директория с образами: %s", rootDir))
	} else {
		log.Info(fmt.Sprintf("установлена корневая директория с образами: %s", rootDir))
	}

	// Контекст, завершаемый по SIGINT (Ctrl+C) или SIGTERM.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// errgroup — запуск воркеров с общим контекстом.
	// Если любой воркер вернёт ошибку, контекст группы отменяется,
	// и все остальные воркеры получают сигнал завершения.
	g, ctxGroup := errgroup.WithContext(ctx)

	// Список всех воркеров, соответствующих интерфейсу models.Workerses.
	var workers []models.Workerses

	// Инициализация watcherWorker.
	pollInterval, _ := cmd.Flags().GetDuration(FlagPollInterval)
	changeFiles := make(chan models.FileEvent, watcher.DefaultChangeFiles)

	watcherWorker, err := watcher.NewWatcher(&watcher.Config{
		Log:          log,
		PollInterval: pollInterval,
		RootDir:      rootDir,
		ChangeFiles:  changeFiles,
	})
	if err != nil {
		log.Error("не удалось создать процесс отслеживания директории с образами", slog.Any("error", err))

		return
	}
	workers = append(workers, watcherWorker)

	// Инициализация repoWorker.
	changeRepo := make(chan models.RepoEvent, repo.DefaultChangeRepos)

	repoWorker, err := repo.NewRepo(&repo.Config{
		Log:         log,
		ChangeFiles: changeFiles,
		ChangeRepos: changeRepo,
	})
	if err != nil {
		log.Error("не удалось создать процесс отслеживания репозиториев", slog.Any("error", err))

		return
	}
	workers = append(workers, repoWorker)

	// Инициализация webWorker.
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(logging.NewGin(log))
	router.Use(gin.Recovery())

	port, _ := cmd.Flags().GetInt(FlagPort)

	webWorker, err := web.NewWeb(&web.Config{
		Log:         log,
		Port:        port,
		Router:      router,
		ChangeRepos: changeRepo,
		RootDir:     rootDir,
		Copyright:   copyright,
		Version:     strings.ReplaceAll(version, "v", ""),
	})
	if err != nil {
		log.Error("не удалось создать веб-сервер", slog.Any("error", err))

		return
	}
	workers = append(workers, webWorker)

	// Запуск каждого воркера в отдельной горутине errgroup.
	for _, w := range workers {
		w := w // захват переменной для замыкания
		g.Go(func() error {
			return w.Run(ctxGroup)
		})
	}

	// Ожидание завершения всех воркеров.
	// Если хотя бы один воркер вернул ошибку, err будет не nil.
	err = g.Wait()
	if err != nil {
		log.Error("программа завершена с ошибкой", slog.Any("error", err))

		return
	}

	log.Info("программа завершила работу")
}
