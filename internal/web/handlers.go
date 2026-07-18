package web

import (
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/kirsrus/iso2repo/models"
)

// sortRepoViews сортирует список repoView по имени в алфавитном порядке.
func sortRepoViews(repos []repoView) {
	sort.Slice(repos, func(i, j int) bool {
		return strings.ToLower(repos[i].Name) < strings.ToLower(repos[j].Name)
	})
}

// sourcesTxtData модель данных для шаблона sources.txt (текстовый вывод для curl/wget).
type sourcesTxtData struct {
	// CustomComment — пустой комментарий, который можно будет заполнить позже.
	CustomComment string
	// Repos — список строк RepoString() для всех репозиториев.
	Repos []string
	// Версия программы.
	Version string
	// ServerIP — IP-адрес интерфейса сервера, к которому обратился клиент.
	ServerIP string
	// Port — номер порта, на котором слушает веб-интерфейс.
	Port int
}

// repoView модель для отображения репозитория в шаблоне.
type repoView struct {
	Name      string
	Path      string
	Type      string
	TypeLabel string
}

// addressView модель для отображения IP-адреса с ссылкой на sources.list.
type addressView struct {
	Address string
	URL     string
}

// indexData модель данных для шаблона index.html.
type indexData struct {
	Repos     []repoView
	Addresses []addressView
	RootDir   string
	Copyright string
	Version   string
}

// sourcesData модель данных для шаблона sources.html.
type sourcesData struct {
	// Text содержит готовый текст для отображения в <pre>,
	// где каждая строка начинается с "# ".
	Text string
}

// entryView модель для отображения элемента (файла/директории) внутри репозитория.
type entryView struct {
	Name  string
	URL   string
	IsDir bool
	Size  string
}

// crumb модель элемента "хлебных крошек".
type crumb struct {
	Name string
	URL  string
}

// repoData модель данных для шаблона repo.html.
type repoData struct {
	RepoName    string
	Breadcrumbs []crumb
	Entries     []entryView
}

// staticData модель данных для шаблона static.html.
type staticData struct {
	Title       string
	Breadcrumbs []crumb
	Entries     []entryView
}

// newIndexData создаёт indexData из sync.Map репозиториев.
func newIndexData(repos *sync.Map, rootDir, copyright, version string, port int) indexData {
	data := indexData{
		Repos:     make([]repoView, 0),
		Addresses: getLocalAddresses(port),
		RootDir:   rootDir,
		Copyright: copyright,
		Version:   version,
	}

	repos.Range(func(_, value any) bool {
		repo, ok := value.(models.Repoes)
		if !ok {
			return true
		}

		meta := repo.Metadata()
		typeLabel := ""
		typeStr := ""

		switch meta.Type {
		case models.RepoISO:
			typeLabel = "ISO"
			typeStr = "ISO"
		case models.RepoExtracted:
			typeLabel = "Распакованный"
			typeStr = "Extracted"
		case models.RepoCustom:
			typeLabel = "Пользовательский"
			typeStr = "Custom"
		}

		data.Repos = append(data.Repos, repoView{
			Name:      meta.Name,
			Path:      meta.Path,
			Type:      typeStr,
			TypeLabel: typeLabel,
		})

		return true
	})

	sortRepoViews(data.Repos)

	return data
}

// getServerIP определяет IP-адрес интерфейса сервера, к которому обратился клиент.
// Сначала пытается извлечь IP из c.Request.Host (если там IP, а не домен).
// Если Host содержит домен — использует address из query-параметра (если это IP),
// иначе возвращает первый не-loopback локальный IP.
func getServerIP(c *gin.Context, address string) string {
	// Пробуем извлечь IP из Host (отсекаем порт)
	host := c.Request.Host
	if host != "" {
		// Отсекаем порт, если есть
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
		// Проверяем, является ли host IP-адресом
		if ip := net.ParseIP(host); ip != nil && ip.To4() != nil {
			return ip.String()
		}
	}

	// Если address из query — это IP, используем его
	if ip := net.ParseIP(address); ip != nil && ip.To4() != nil {
		return address
	}

	// Иначе берём первый не-loopback локальный IPv4 адрес
	interfaces, err := net.Interfaces()
	if err != nil {
		return "127.0.0.1"
	}

	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 {
			continue
		}
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil || ip.To4() == nil {
				continue
			}
			return ip.String()
		}
	}

	return "127.0.0.1"
}

// newSourcesTxtData создаёт sourcesTxtData из sync.Map репозиториев.
// Формирует список строк RepoString() с заменой 0.0.0.0 на указанный address:port.
func newSourcesTxtData(repos *sync.Map, address string, port int, version, serverIP string) sourcesTxtData {
	data := sourcesTxtData{
		CustomComment: "",
		Repos:         make([]string, 0),
		Version:       version,
		ServerIP:      serverIP,
		Port:          port,
	}

	repos.Range(func(_, value any) bool {
		repo, ok := value.(models.Repoes)
		if !ok {
			return true
		}

		repoStr := repo.RepoString()
		if repoStr == "" {
			return true
		}

		// Заменяем 0.0.0.0 на указанный адрес:порт
		repoStr = strings.ReplaceAll(repoStr, "0.0.0.0", fmt.Sprintf("%s:%d", address, port))

		data.Repos = append(data.Repos, repoStr)

		return true
	})

	// Сортируем строки для стабильного вывода
	sort.Strings(data.Repos)

	return data
}

// getLocalAddresses возвращает список уникальных IP-адресов локальной машины
// в порядке: repo.loc, 127.0.0.1, остальные адреса (отсортированные по возрастанию).
func getLocalAddresses(port int) []addressView {
	addresses := make([]addressView, 0)

	// repo.loc — всегда первым
	addresses = append(addresses, addressView{
		Address: "repo.loc",
		URL:     fmt.Sprintf("/sources.list?address=repo.loc&port=%d", port),
	})

	// 127.0.0.1 — всегда вторым
	addresses = append(addresses, addressView{
		Address: "127.0.0.1",
		URL:     fmt.Sprintf("/sources.list?address=127.0.0.1&port=%d", port),
	})

	// Получаем все интерфейсы
	interfaces, err := net.Interfaces()
	if err != nil {
		return addresses
	}

	others := make([]string, 0)

	for _, iface := range interfaces {
		// Пропускаем неработающие интерфейсы
		if iface.Flags&net.FlagUp == 0 {
			continue
		}
		// Пропускаем loopback — он уже добавлен как 127.0.0.1
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}

			// Пропускаем IPv6 и nil
			if ip == nil || ip.To4() == nil {
				continue
			}

			ipStr := ip.String()
			// Пропускаем 127.0.0.1 — уже добавлен
			if ipStr == "127.0.0.1" {
				continue
			}

			others = append(others, ipStr)
		}
	}

	// Сортируем остальные адреса для стабильного вывода
	sort.Strings(others)

	for _, ip := range others {
		addresses = append(addresses, addressView{
			Address: ip,
			URL:     fmt.Sprintf("/sources.list?address=%s&port=%d", ip, port),
		})
	}

	return addresses
}

// registerRoutes регистрирует обработчики HTTP запросов.
func (m *Web) registerRoutes() {
	m.router.GET("/", m.handleIndex)
	m.router.GET("/favicon.ico", m.handleFavicon)
	m.router.GET("/logo.gif", m.handleLogo)
	m.router.GET("/sources.list", m.handleSources)
	m.router.GET("/repo/*path", m.handleRepo)
	m.router.GET("/static/*path", m.handleStatic)
}

// handleIndex обработчик корневого маршрута.
// В зависимости от User-Agent отдаёт разный контент:
//   - curl/wget — text/plain с закомментированным списком репозиториев для repo.loc;
//   - браузер — HTML-страница index.html.
func (m *Web) handleIndex(c *gin.Context) {
	userAgent := c.GetHeader("User-Agent")

	// Определяем, является ли клиент curl или wget
	isCurlOrWget := strings.HasPrefix(userAgent, "curl/") || strings.HasPrefix(userAgent, "Wget/")

	if isCurlOrWget {
		serverIP := getServerIP(c, "repo.loc")
		data := newSourcesTxtData(&m.repos, "repo.loc", m.port, m.version, serverIP)

		// Рендерим шаблон sources.txt в буфер и отдаём как text/plain
		buf := new(strings.Builder)
		if err := m.templates.ExecuteTemplate(buf, "sources.txt", data); err != nil {
			m.log.Error("ошибка рендеринга шаблона sources.txt", slog.String("error", err.Error()))
			c.Status(http.StatusInternalServerError)
			return
		}

		c.Data(http.StatusOK, "text/plain; charset=utf-8", []byte(buf.String()))
		return
	}

	data := newIndexData(&m.repos, m.rootDir, m.copyright, m.version, m.port)
	c.HTML(http.StatusOK, "index.html", data)
}

// handleFavicon обработчик маршрута /favicon.ico.
// Отдаёт встроенный файл favicon.ico из templatesFS.
func (m *Web) handleFavicon(c *gin.Context) {
	faviconData, err := templatesFS.ReadFile("templates/favicon.ico")
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	c.Data(http.StatusOK, "image/x-icon", faviconData)
}

// handleLogo обработчик маршрута /logo.gif.
// Отдаёт встроенный файл logo.gif из templatesFS.
func (m *Web) handleLogo(c *gin.Context) {
	logoData, err := templatesFS.ReadFile("templates/logo.gif")
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	c.Data(http.StatusOK, "image/gif", logoData)
}

// handleSources обработчик маршрута /sources.list.
// Формирует список строк RepoString() для всех репозиториев.
// По умолчанию адрес 0.0.0.0 заменяется на repo.loc.
// Параметр ?address=192.168.10.1 позволяет указать произвольный адрес.
// В зависимости от User-Agent отдаёт разный контент:
//   - curl/wget — text/plain с закомментированным списком репозиториев;
//   - браузер — HTML-страница sources.html.
func (m *Web) handleSources(c *gin.Context) {
	// Определяем адрес для подстановки вместо 0.0.0.0
	address := c.DefaultQuery("address", "repo.loc")

	userAgent := c.GetHeader("User-Agent")

	// Определяем, является ли клиент curl или wget
	isCurlOrWget := strings.HasPrefix(userAgent, "curl/") || strings.HasPrefix(userAgent, "Wget/")

	if isCurlOrWget {
		serverIP := getServerIP(c, address)
		data := newSourcesTxtData(&m.repos, address, m.port, m.version, serverIP)

		// Рендерим шаблон sources.txt в буфер и отдаём как text/plain
		buf := new(strings.Builder)
		if err := m.templates.ExecuteTemplate(buf, "sources.txt", data); err != nil {
			m.log.Error("ошибка рендеринга шаблона sources.txt", slog.String("error", err.Error()))
			c.Status(http.StatusInternalServerError)
			return
		}

		c.Data(http.StatusOK, "text/plain; charset=utf-8", []byte(buf.String()))
		return
	}

	lines := make([]string, 0)

	m.repos.Range(func(_, value any) bool {
		repo, ok := value.(models.Repoes)
		if !ok {
			return true
		}

		repoStr := repo.RepoString()
		if repoStr == "" {
			return true
		}

		// Заменяем 0.0.0.0 на указанный адрес:порт
		repoStr = strings.ReplaceAll(repoStr, "0.0.0.0", fmt.Sprintf("%s:%d", address, m.port))

		lines = append(lines, repoStr)

		return true
	})

	// Сортируем строки для стабильного вывода
	sort.Strings(lines)

	// Формируем единый текст: каждая строка с префиксом "#" и переводом строки
	text := "#" + strings.Join(lines, "\n#")

	data := sourcesData{
		Text: text,
	}

	c.HTML(http.StatusOK, "sources.html", data)
}

// handleRepo обработчик маршрута /repo/*path.
// Первый сегмент пути — имя репозитория.
// Если путь указывает на директорию — отображается содержимое.
// Если путь указывает на файл — файл отдаётся на скачивание.
func (m *Web) handleRepo(c *gin.Context) {
	// Полный путь после /repo/, например "/axxon-repo-4.5.10.594-2023-12-12.iso/dists/"
	fullPath := c.Param("path")
	// Убираем ведущий слеш
	fullPath = strings.TrimPrefix(fullPath, "/")

	// Разбиваем на сегменты
	parts := strings.SplitN(fullPath, "/", 2)
	repoName := parts[0]

	if repoName == "" {
		c.Status(http.StatusNotFound)
		return
	}

	// Ищем репозиторий по имени
	value, ok := m.repos.Load(repoName)
	if !ok {
		c.Status(http.StatusNotFound)
		return
	}

	repo, ok := value.(models.Repoes)
	if !ok {
		c.Status(http.StatusNotFound)
		return
	}

	// Определяем путь внутри репозитория (без имени репозитория)
	innerPath := ""
	if len(parts) > 1 {
		innerPath = strings.TrimSuffix(parts[1], "/")
	}

	// Получаем список содержимого директории
	entries, err := repo.List(c.Request.Context(), innerPath)
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}

	if len(entries) == 0 && innerPath != "" {
		// Возможно, это файл — проверяем его существование через родительскую директорию
		parentDir := path.Dir(innerPath)
		fileName := path.Base(innerPath)

		// Для корня родительская директория — пустая строка
		if parentDir == "." {
			parentDir = ""
		}

		parentEntries, err := repo.List(c.Request.Context(), parentDir)
		if err != nil {
			c.Status(http.StatusNotFound)
			return
		}

		// Ищем файл в списке родительской директории
		fileFound := false
		for _, entry := range parentEntries {
			if !entry.IsDir && entry.Name == fileName {
				fileFound = true
				break
			}
		}

		if !fileFound {
			c.Status(http.StatusNotFound)
			return
		}

		// Файл существует — открываем и отдаём на скачивание
		reader, err := repo.Open(c.Request.Context(), innerPath)
		if err == nil && reader != nil {
			defer reader.Close()
			// Определяем имя файла из пути
			c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", fileName))
			c.DataFromReader(http.StatusOK, -1, "application/octet-stream", reader, nil)
			return
		}

		// Если не удалось открыть как файл — 404
		c.Status(http.StatusNotFound)
		return
	}

	// Формируем "хлебные крошки"
	breadcrumbs := makeBreadcrumbs(repoName, innerPath)

	// Формируем список entryView с сортировкой: директории сверху, файлы снизу
	entryViews := makeEntryViews(repoName, innerPath, entries)

	data := repoData{
		RepoName:    repoName,
		Breadcrumbs: breadcrumbs,
		Entries:     entryViews,
	}

	c.HTML(http.StatusOK, "repo.html", data)
}

// makeBreadcrumbs формирует список "хлебных крошек" для навигации.
// Имя репозитория всегда является кликабельной ссылкой на /repo/<repoName>/.
func makeBreadcrumbs(repoName, innerPath string) []crumb {
	result := make([]crumb, 0)

	// Имя репозитория — всегда ссылка на корень репозитория
	result = append(result, crumb{
		Name: repoName,
		URL:  "/repo/" + repoName + "/",
	})

	if innerPath == "" {
		return result
	}

	segments := strings.Split(innerPath, "/")
	currentPath := ""

	for i, seg := range segments {
		if seg == "" {
			continue
		}
		if i == len(segments)-1 {
			// Последний сегмент — текущая директория (без ссылки)
			result = append(result, crumb{Name: seg})
		} else {
			currentPath += "/" + seg
			result = append(result, crumb{
				Name: seg,
				URL:  "/repo/" + repoName + currentPath + "/",
			})
		}
	}

	return result
}

// makeEntryViews формирует отсортированный список entryView из models.Entry.
// Директории всегда вверху, файлы внизу. Внутри каждой группы — сортировка по имени.
func makeEntryViews(repoName, innerPath string, entries []models.Entry) []entryView {
	dirs := make([]entryView, 0)
	files := make([]entryView, 0)

	baseURL := "/repo/" + repoName
	if innerPath != "" {
		baseURL += "/" + innerPath
	}

	for _, e := range entries {
		url := baseURL + "/" + e.Name
		if e.IsDir {
			url += "/"
		}

		ev := entryView{
			Name:  e.Name,
			URL:   url,
			IsDir: e.IsDir,
			Size:  formatSize(e.Size),
		}

		if e.IsDir {
			dirs = append(dirs, ev)
		} else {
			files = append(files, ev)
		}
	}

	// Сортируем каждую группу по имени
	sort.Slice(dirs, func(i, j int) bool {
		return strings.ToLower(dirs[i].Name) < strings.ToLower(dirs[j].Name)
	})
	sort.Slice(files, func(i, j int) bool {
		return strings.ToLower(files[i].Name) < strings.ToLower(files[j].Name)
	})

	// Объединяем: директории сверху, файлы снизу
	return append(dirs, files...)
}

// formatSize форматирует размер файла в человекочитаемый вид.
func formatSize(size int64) string {
	if size == 0 {
		return ""
	}

	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}

	div, exp := int64(unit), 0
	for n := size / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}

	return fmt.Sprintf("%.1f %cB", float64(size)/float64(div), "KMGTPE"[exp])
}

// handleStatic обработчик маршрута /static/*path.
// Отображает содержимое директорий из rootDir и отдаёт файлы для просмотра в браузере.
func (m *Web) handleStatic(c *gin.Context) {
	// Путь после /static/, например "/images/logo.png"
	reqPath := c.Param("path")
	reqPath = strings.TrimPrefix(reqPath, "/")

	// Полный путь в файловой системе
	fullPath := filepath.Join(m.rootDir, reqPath)

	// Проверяем, что путь находится внутри rootDir (безопасность)
	absRoot, _ := filepath.Abs(m.rootDir)
	absFull, _ := filepath.Abs(fullPath)
	if !strings.HasPrefix(absFull, absRoot) {
		c.Status(http.StatusNotFound)
		return
	}

	// Проверяем, существует ли путь
	info, err := os.Stat(fullPath)
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}

	// Если это файл — отдаём его для просмотра в браузере (без Content-Disposition: attachment)
	if !info.IsDir() {
		c.File(fullPath)
		return
	}

	// Это директория — читаем её содержимое
	entries, err := os.ReadDir(fullPath)
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}

	// Формируем "хлебные крошки"
	breadcrumbs := makeStaticBreadcrumbs(reqPath)

	// Формируем список entryView
	entryViews := makeStaticEntryViews(reqPath, entries, info)

	// Определяем заголовок
	title := "Файлы"
	if reqPath != "" {
		title = filepath.Base(reqPath)
	}

	data := staticData{
		Title:       title,
		Breadcrumbs: breadcrumbs,
		Entries:     entryViews,
	}

	c.HTML(http.StatusOK, "static.html", data)
}

// makeStaticBreadcrumbs формирует "хлебные крошки" для навигации по файловой системе.
func makeStaticBreadcrumbs(reqPath string) []crumb {
	result := make([]crumb, 0)

	if reqPath == "" {
		return result
	}

	segments := strings.Split(reqPath, string(filepath.Separator))
	// Также поддерживаем слеши URL
	if len(segments) == 1 {
		segments = strings.Split(reqPath, "/")
	}

	currentPath := ""

	for i, seg := range segments {
		if seg == "" {
			continue
		}
		if i == len(segments)-1 {
			// Последний сегмент — текущая директория (без ссылки)
			result = append(result, crumb{Name: seg})
		} else {
			currentPath += "/" + seg
			result = append(result, crumb{
				Name: seg,
				URL:  "/static" + currentPath + "/",
			})
		}
	}

	return result
}

// makeStaticEntryViews формирует отсортированный список entryView из fs.DirEntry.
// Директории всегда вверху, файлы внизу. Внутри каждой группы — сортировка по имени.
func makeStaticEntryViews(reqPath string, entries []fs.DirEntry, parentInfo os.FileInfo) []entryView {
	dirs := make([]entryView, 0)
	files := make([]entryView, 0)

	baseURL := "/static"
	if reqPath != "" {
		baseURL += "/" + reqPath
	}

	for _, e := range entries {
		// Пропускаем скрытые файлы
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}

		url := baseURL + "/" + e.Name()
		isDir := e.IsDir()
		if isDir {
			url += "/"
		}

		var size int64
		if !isDir {
			info, err := e.Info()
			if err == nil {
				size = info.Size()
			}
		}

		ev := entryView{
			Name:  e.Name(),
			URL:   url,
			IsDir: isDir,
			Size:  formatSize(size),
		}

		if isDir {
			dirs = append(dirs, ev)
		} else {
			files = append(files, ev)
		}
	}

	// Сортируем каждую группу по имени
	sort.Slice(dirs, func(i, j int) bool {
		return strings.ToLower(dirs[i].Name) < strings.ToLower(dirs[j].Name)
	})
	sort.Slice(files, func(i, j int) bool {
		return strings.ToLower(files[i].Name) < strings.ToLower(files[j].Name)
	})

	// Объединяем: директории сверху, файлы снизу
	return append(dirs, files...)
}
