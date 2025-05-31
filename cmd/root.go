package cmd

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/gin-contrib/static"
	"github.com/gin-gonic/gin"
	"github.com/juju/errors"
	"github.com/kirsrus/iso2repo/pkg/logging"
	"github.com/kirsrus/iso2repo/pkg/sevenz"
	"github.com/kirsrus/iso2repo/pkg/tools"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cast"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/thoas/go-funk"
	ginlogrus "github.com/toorop/gin-logrus"
	prefixed "github.com/x-cray/logrus-prefixed-formatter"
)

const (
	binName   = "iso2repo"
	product   = "iso2repo"
	copyright = "Стерликов Кирилл"

	// Секунды, через которые программа завершает работу после вывода информации об ошибке.
	sleepBeforeErrorExit = 3
)

var (
	cfgFile   string
	globalLog *logrus.Logger

	gitVersion string
	gitCommit  string
	gitDate    string

	// Ключ, который указывает, что при ошибке будет выводиться только лог (true),
	// или информация в свободной форме (false)
	onlyLog bool

	// Шаблоны отображения в WEB-интерфейсе
	templates Template
)

var rootCmd = &cobra.Command{
	Use:   "iso2repo",
	Short: "Создаёт локальные репозитории из ISO-образов",
	Long: `Программа решает задачу возможности подключения к репозиториям при отсутствии в локальной сети Интернета (изолированная сеть). Репозитории заранее скачивается и упаковываются в ISO-образа на компьютере, подключённый Интернету и через USB носитель переносится на один из компьютеров в изолированной локальной сети. Далее в папке с этими ISO-образами запускается эта программа, которая делает из них полноценные сетевые HTTP репозитории, которые можно подключить к участникам сети. Один ISO-образ, один репозиторий. Так же программа позволяет просматривать любые файлы (включая сами ISO-образы) во вложенных папках через встроенный WEB-сервер и удобно скачивать их участниками сети через wget/curl.

Примеры использования:

Публикация всех ISO-образов в текущей директории и всех поддиректорией:
   iso2repo

Публикация конкретного ISO-репозитория:
   iso2repo --iso d:/repo.iso

Публикация всех ISO-образов в текущей директории и всех поддиректорией папки dir:
   iso2repo --dir d:/repo_dir


`,
	SilenceErrors: true, // Отключает вывод описния ошибок
	SilenceUsage:  true, // Отключает вывод текста "описание использования" при ошибке
	RunE:          rootRunE,
}

// Execute добавляет все дочерние команды к корневой команде и устанавливает соответствующие флаги.
// Это вызывается main.main(). Это должно произойти только один раз с rootCmd.
func Execute(gitVersionIn string, gitCommitIn string, gitDateIn string, templatesIn Template) {
	gitVersion = gitVersionIn
	gitCommit = gitCommitIn
	gitDate = strings.ReplaceAll(gitDateIn, "T", " ")
	templates = templatesIn

	err := rootCmd.Execute()
	if err != nil {
		if onlyLog {
			if globalLog.Level > logrus.InfoLevel {
				globalLog.WithFields(map[string]interface{}{
					"stack": errors.ErrorStack(errors.Annotate(err, "end point")),
				}).Error(err.Error())
			} else {
				globalLog.Error(err.Error())
			}
		} else {
			// Убираем дублирующуюся первую строку, если она соответствует имени ошибки
			stack := strings.Split(fmt.Sprintf("%+v", errors.ErrorStack(errors.Trace(err))), "\n")
			if len(stack) > 0 && stack[0] == errors.Cause(err).Error() {
				stack = stack[1:]
			}
			for i := range stack {
				stack[i] = strings.Trim(stack[i], ": ")
			}

			fmt.Printf("ERROR: %s\nSTACK:\n  ", errors.Cause(err))
			fmt.Printf("%s\n\n", strings.Join(stack, "\n  "))
		}
		time.Sleep(sleepBeforeErrorExit * time.Second)
		os.Exit(1)
	}
}

func init() {
	initLogging()
	cobra.OnInitialize(initGlobalConfig)

	rootCmd.PersistentFlags().Bool("version", false, "версия программы")
	rootCmd.PersistentFlags().String("config", "", "файл конфигурации (по умолчанию $HOME/.iso2repo.yaml)")
	rootCmd.PersistentFlags().String("log", "", "файл логирования")
	rootCmd.PersistentFlags().String("level", "", "уровень логирования (debug|info|warn|error)")
	rootCmd.PersistentFlags().String("iso", "", "конкретный ISO-образ репозитория")
	rootCmd.PersistentFlags().String("dir", "", "корневая директория со всеми ISO-образами репозиториев")
	rootCmd.PersistentFlags().Int("port", 4309, "порт WEB-сервера доступа к репозиториям")
}

// initGlobalConfig reads in config file and ENV variables if set.
func initGlobalConfig() {
	if cfgFile != "" {
		// Use config file from the flag.
		viper.SetConfigFile(cfgFile)
	} else {
		// Find home directory.
		home, err := os.UserHomeDir()
		cobra.CheckErr(err)

		// Search config in home directory with name ".iso2repo" (without extension).
		viper.AddConfigPath(home)
		viper.SetConfigType("yaml")
		viper.SetConfigName(".iso2repo")
	}

	viper.AutomaticEnv() // read in environment variables that match

	// If a config file is found, read it in.
	if err := viper.ReadInConfig(); err == nil {
		_, _ = fmt.Fprintln(os.Stderr, "Using config file:", viper.ConfigFileUsed())
	}
}

func initLogging() {
	// Определяем версию Windows, которая не поддерживает цвет в консоли и выключаем его
	coloredLog := true
	osName, osMajor, osMinor, _ := tools.OsVersion()
	if osName == "windows" && osMajor <= 6 && osMinor <= 1 { // Windows 7, 2008 и ниже
		coloredLog = false
	}

	globalLog = logrus.New()
	globalLog.Level = logrus.InfoLevel
	globalLog.Out = io.Discard

	if osName == "windows" {
		globalLog.Formatter = &logrus.TextFormatter{
			ForceColors: coloredLog,
		}
	} else {
		globalLog.Formatter = &prefixed.TextFormatter{
			DisableColors: !coloredLog,
		}
	}
	globalLog.AddHook(logging.LogrusContextHook{})
}

func rootRunE(cmd *cobra.Command, _ []string) error {
	var err error
	gitVersion = strings.TrimSpace(gitVersion)
	onlyLog = true

	if cmd.Flag("version").Changed {
		fmt.Printf("%s\n", gitVersion)
		return nil
	}

	// Порт WEB-сервера
	webPort := cast.ToInt(cmd.Flag("port").Value.String())

	// Настройка логирования

	log := globalLog
	log.Level = logrus.InfoLevel
	log.Out = os.Stdout

	logFileRaw := cmd.Flag("log").Value.String()
	if logFileRaw != "" {
		logFile, err := os.OpenFile(logFileRaw, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0600)
		if err != nil {
			log.Warnf("ошибка открытия файла для сохранения лога '%s'", logFileRaw)
		} else {
			log.Formatter.(*prefixed.TextFormatter).ForceColors = false
			log.Out = io.MultiWriter(os.Stdout, logFile)
		}
	}

	if cmd.Flag("level").Value.String() != "" {
		logLevelRaw := cmd.Flag("level").Value.String()
		if l, err := logrus.ParseLevel(logLevelRaw); err != nil {
			log.Warnf("неправильно указан уровень логирования '%s'", cmd.Flag("level").Value.String())
		} else {
			log.Level = l
		}
	}

	if log.Level > logrus.InfoLevel {
		log.Formatter.(*prefixed.TextFormatter).FullTimestamp = false
	}

	currLolLevel := log.Level
	log.Level = logrus.InfoLevel

	interfaces, err := net.Interfaces()
	if err != nil {
		return errors.Trace(err)
	}

	logText := []string{
		fmt.Sprintf("%s %s (%s) %s @ %s", product, gitVersion, gitCommit[0:7], gitDate, copyright),
		"",
		"Создание локального HTTP репозитория из ISO-файлов. Для работы необходим установленный",
		fmt.Sprintf("пакет 7z. Узнать все доступные опции программы можно через запуск '%s --help'", binName),
		"",
	}

	if cmd.Flag("dir").Value.String() != "" {
		logText = append(logText, fmt.Sprintf("Корневая директория поиска ISO-образов '%s'", cmd.Flag("dir").Value.String()))
	}

	// Описание hosts

	logText = append(logText,
		"",
		"(опционально) Добавить в /etc/hosts разрешение локального домена на нужный IP-адрес:",
		"",
		"  `sudo nano /etc/hosts`",
		"  x.x.x.x repo.loc repo",
		"",
	)

	// Ссылка на WEB-интерфейс

	logText = append(logText,
		"Доступ к WEB-интерфейсу:",
		"",
	)

	logText = append(logText, fmt.Sprintf("  http://repo.loc:%d", webPort))
	for _, v := range interfaces {
		if v.Name == "lo" {
			continue
		}

		addrs, err := v.Addrs()
		if err != nil {
			_, _ = fmt.Fprint(os.Stderr, err.Error())
			continue
		}

		for _, z := range addrs {

			if ipRaw, ok := z.(*net.IPNet); !ok {
				_, _ = fmt.Fprintf(os.Stderr, "error decode addr %v", z)
				continue
			} else if ipRaw.IP.IsPrivate() {
				logText = append(logText, fmt.Sprintf("  http://%s:%d", ipRaw.IP.To4().String(), webPort))
			}
		}
	}

	// Ссылки на скачивание списка репозиториев

	logText = append(logText,
		"",
		"Установить список репозиторий в локальный source.list можно командой:",
		"",
	)

	// Список IP-адресов сервера
	IPList := make([]string, 0)

	logText = append(logText, fmt.Sprintf("  sudo wget repo.loc:%d/sources.list?ip=repo.loc -O /etc/apt/sources.list.d/iso2repo.list", webPort))
	IPList = append(IPList, "repo.loc")
	for _, v := range interfaces {
		if v.Name == "lo" {
			continue
		}

		addrs, err := v.Addrs()
		if err != nil {
			_, _ = fmt.Fprint(os.Stderr, err.Error())
			continue
		}

		for _, z := range addrs {

			if ipRaw, ok := z.(*net.IPNet); !ok {
				_, _ = fmt.Fprintf(os.Stderr, "error decode addr %v", z)
				continue
			} else if ipRaw.IP.IsPrivate() {
				logText = append(logText, fmt.Sprintf("  sudo wget %s:%d/sources.list -O /etc/apt/sources.list.d/iso2repo.list", ipRaw.IP.To4().String(), webPort))
				IPList = append(IPList, ipRaw.IP.To4().String())
			}
		}

	}

	for _, v := range tools.LogInfoWidget(logText, "*") {
		log.Info(v)
	}
	log.Level = currLolLevel

	// Проверка наличия утилиты 7z

	if _, err := sevenz.Find7Z(); err != nil {
		return errors.Trace(err)
	}

	// Чтение ISO-образов

	var isoFiles []string

	isoOnceFile := cmd.Flag("iso").Value.String()
	if isoOnceFile != "" {
		if _, err := os.Stat(isoOnceFile); os.IsNotExist(err) {
			return errors.Errorf("не удалось найти указанный ISO-файл '%s'", isoOnceFile)
		}
		log.Infof("обнаружен ISO-образ '%s'", filepath.Base(isoOnceFile))
		isoFiles = []string{isoOnceFile}
	} else {
		isoManyFile := cmd.Flag("dir").Value.String()
		if isoManyFile == "" {
			isoManyFile = filepath.Dir(os.Args[0])
		}
		isoFiles, err = sevenz.FindISOInDir(isoManyFile)
		if err != nil {
			return errors.Trace(err)
		} else if len(isoFiles) == 0 {
			log.Infof("указать папку с образами можно через запуск '%s --dir <путь_к_папке_с_ISO>'", binName)
			log.Warnf("не обнаружено ISO-образов в директории '%s'", isoManyFile)
		}

		for _, v := range isoFiles {
			log.WithField("prefix", "поиск ISO-образов").Infof("обнаружен '%s'", filepath.Base(v))
		}
	}

	isoDates := make([]sevenz.SevenZ, 0)
	for _, isoFile := range isoFiles {
		isoData, err := sevenz.NewSevenZ(isoFile, log)
		if err != nil {
			if err.Error() == sevenz.ErrIsNotRepo.Error() {
				log.WithField("prefix", "отбраковка ISO-образов").Infof("'%s' не является репозиторием", filepath.Base(isoFile))
			} else {
				log.Warnf("ошбика открытия образа %s: %s", filepath.Base(isoFile), err.Error())
			}
		} else {
			isoDates = append(isoDates, *isoData)
		}
	}

	if len(isoDates) == 0 {
		log.Warnf("во всех %d ISO-файлах репозиториев не обнаружено", len(isoFiles))
	}

	// WEB-сервер

	if webPort == 0 {
		return errors.Errorf("указан некорректный порт WEB-сервера '%s'", cmd.Flag("port").Value.String())
	}

	gin.SetMode(gin.ReleaseMode)
	webRouter := gin.New()
	if log.Level > logrus.InfoLevel {
		webRouter.Use(ginlogrus.Logger(log))
	}
	webRouter.Use(gin.Recovery())
	webRouter.Use(func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization")
		c.Next()
	})

	// Хэндлеры

	webRouter.GET("/", func(c *gin.Context) {

		data := TemplateIndex{
			Title:      fmt.Sprintf("iso2repo (%s) от %s", gitVersion, gitDate),
			Version:    gitVersion,
			Date:       gitDate,
			Copyright:  copyright,
			ISOPathLen: 0,
			ISOPath:    make([]string, 0),
			IPList:     IPList,
		}

		for _, s := range isoDates {
			data.ISOPath = append(data.ISOPath, filepath.Base(s.ISOPath))
		}
		data.ISOPathLen = len(data.ISOPath)

		tpl, err := template.New("index").Parse(templates.Index)
		if err != nil {
			c.String(http.StatusInternalServerError, err.Error())
			return
		}

		err = tpl.ExecuteTemplate(c.Writer, "index", data)
		if err != nil {
			c.String(http.StatusInternalServerError, err.Error())
			return
		}
	})

	webRouter.GET("/repo", func(c *gin.Context) {

		c.Redirect(http.StatusPermanentRedirect, "/")
	})

	webRouter.GET("/sources.list", func(c *gin.Context) {
		data := TemplateSources{
			Title:     "Sources List",
			Version:   gitVersion,
			Date:      gitDate,
			Copyright: copyright,
			DebPath:   make([]string, 0),
			IPList:    IPList,
		}

		IP := c.Query("ip")

		if IP == "" {
			if len(IPList) > 0 {
				IP = IPList[0]
			} else {
				IP = "127.0.0.1"
			}
		}

		for _, v := range isoDates {
			itemMsg := v.GetRepoString()

			// Добавляем принудительный параметр x64, чтобы не было попытки получить x32 данные из репозитория
			itemMsg = strings.Replace(itemMsg, "deb", "deb [arch=amd64]", 1)

			addr := fmt.Sprintf("http://%s:%d", IP, webPort)
			itemMsg = strings.Replace(itemMsg, "http://0.0.0.0", addr, 1)
			data.DebPath = append(data.DebPath, itemMsg)
		}

		// Если запрос идёт не из браузера, а из wget или curl - отдавать только текст.
		// Инече сформированную HTML таблицу

		userAgent := c.Request.Header.Get("User-Agent")
		log.Infof("запрос source.list от агента '%s'", userAgent)
		userAgent = strings.ToLower(userAgent)

		if strings.Contains(userAgent, "wget") || strings.Contains(userAgent, "curl") || userAgent == "" {

			// Отдача для wget/curl - только текст

			log.Debug("отдаём список репозиториев в виде TXT")
			c.String(http.StatusOK, strings.Join(data.DebPath, "\n"))
			return

		} else {

			// Отдача для браузера - HTML страница

			log.Debug("отдаём список репозиториев в виде HTML")
			tpl, err := template.New("sources").Parse(templates.Sources)
			if err != nil {
				c.String(http.StatusInternalServerError, err.Error())
				return
			}

			err = tpl.ExecuteTemplate(c.Writer, "sources", data)
			if err != nil {
				c.String(http.StatusInternalServerError, err.Error())
				return
			}

		}
	})

	webRouter.GET("/repo/:iso/*action", func(c *gin.Context) {
		isoFile := c.Param("iso")
		action := c.Param("action")

		data := TemplatesFiles{
			Title:      isoFile,
			Version:    gitVersion,
			Date:       gitDate,
			Copyright:  copyright,
			FilesOrDir: make([]File, 0),
		}

		// Ищем ISO

		resRaw := funk.Find(isoDates, func(v sevenz.SevenZ) bool {
			return strings.Contains(v.ISOPath, isoFile)
		})
		if resRaw == nil {
			c.String(http.StatusNotFound, "образ "+isoFile+" не найден")
			return
		}
		iso := resRaw.(sevenz.SevenZ)

		// Щитаем список файлов и директорий на указанном уровне

		fileInIso, isoFiles, err := iso.ReadPath(action)
		if err != nil {
			c.String(http.StatusNotFound, err.Error())
			return
		}

		// Вывод результата

		if fileInIso != nil {

			// Отдаём файл

			address, _, err := net.SplitHostPort(c.Request.RemoteAddr)
			if err != nil {
				address = c.Request.RemoteAddr
			}

			log.WithField("prefix", address).Infof("отдаётся файл '%s:%s'", isoFile, action)
			c.Writer.WriteHeader(http.StatusOK)
			c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", path.Base(action)))
			c.Header("Content-Type", "application/octet-stream")
			c.Header("Content-Length", fmt.Sprintf("%d", fileInIso.Size))
			err = iso.ReadFile(action, c.Writer)
			if err != nil {
				c.String(http.StatusInternalServerError, err.Error())
				return
			}
			return
		} else {

			// Вывод шаблона с файлами

			data.BackwardURL = File{
				URL: path.Dir(strings.TrimRight(c.Request.RequestURI, "/")) + "/",
			}

			for _, v := range isoFiles {

				if v.IsDir {
					data.FilesOrDir = append(data.FilesOrDir, File{
						IsDir: true,
						Name:  v.Name,
						URL:   v.Name + "/",
					})
				} else {
					data.FilesOrDir = append(data.FilesOrDir, File{
						IsDir: false,
						Name:  v.Name,
						URL:   v.Name,
						Size:  humanize.Bytes(uint64(v.Size)),
					})
				}

			}

			tpl, err := template.New("tree").Parse(templates.Tree)
			if err != nil {
				c.String(http.StatusInternalServerError, err.Error())
				return
			}

			err = tpl.ExecuteTemplate(c.Writer, "tree", data)
			if err != nil {
				c.String(http.StatusInternalServerError, err.Error())
				return
			}

			return
		}
	})

	// Статический контент из папки tools

	rootDir := cmd.Flag("dir").Value.String()
	if rootDir == "" {
		rootDir = filepath.Dir(os.Args[0])
	}
	log.Infof("директория статических файлов: %s", rootDir)
	webRouter.Use(static.Serve("/static/", static.LocalFile(rootDir, true)))

	webServer := &http.Server{
		Addr:    fmt.Sprintf(":%d", webPort),
		Handler: webRouter,
	}

	webQuit := make(chan error)
	go func() {
		log.Infof("WEB-сервер запущен на порту http://127.0.0.1:%d", webPort)
		if err = webServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			webQuit <- err
		}
		webQuit <- nil
	}()

	return <-webQuit
}
