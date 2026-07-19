# iso2repo

HTTP-транслятор локальных APT-репозиториев, упакованных в ISO-образы или простые директории с расширением .iso.

**Поддерживаемые ОС:** Windows 7+ и Linux.

## Описание

Программа решает задачу подключения к локально расположенным APT-репозиториям. Репозитории заранее скачиваются и упаковываются в ISO-образы. Или создаётся папка с расширением `.iso` и туда помещаются файлы с расширением `.deb`. `iso2repo` отслеживает директорию с образами, распознаёт в них структуру APT-репозитория и предоставляет к ним доступ по HTTP в локальной сети.

Поддерживаются три типа репозиториев:

- **ISO-образ** (`.iso`) — стандартный образ с APT-репозиторием внутри.
- **Распакованный ISO** — директория с расширением `.iso`, содержащая распакованную структуру APT-репозитория (с `dists/`, `pool/` и т.д.).
- **Пользовательская папка (custom)** — директория с расширением `.iso`, содержащая `.deb` файлы. Программа динамически генерирует виртуальную структуру APT-репозитория: `Packages`, `Release`, `pool/`.

Кроме того, программа работает как классический статический HTTP-сервер: все файлы и директории из корневого каталога (кроме репозиториев) доступны по адресу `/static/`. Файлы не скачиваются принудительно, а открываются в браузере, если он поддерживает формат — например, PDF, TXT, видео, аудио и любые другие файлы.

## Использование

### Быстрый старт

Самый простой способ — запустить программу без параметров в директории, где лежат ISO-образы или директории с расширением `.iso`:

```bash
iso2repo
```

Все обнаруженные репозитории (`.iso` и директории с расширением `.iso`) станут доступны по HTTP на порту `4309`. Остальные файлы и директории из корневого каталога будут доступны как статический контент по адресу `/static/`.

### Получение списка репозиториев

С удалённого компьютера можно получить список всех обнаруженных репозиториев и инструкции по их подключению:

```bash
curl http://<host>:4309
```

В ответе будут перечислены все доступные репозитории с готовыми строками для добавления в `/etc/apt/sources.list`.

### Запуск с параметрами

```bash
iso2repo [флаги]
```

### Флаги

| Флаг | По умолчанию | Описание |
|------|-------------|----------|
| `--dir` | директория запуска | Корневая директория с ISO-образами и директориями репозиториев |
| `--port` | `4309` | Порт HTTP-сервера |
| `--interval` | `20s` | Интервал опроса директории для обнаружения новых репозиториев или изменений в существующих репозиториях |
| `--level` | `info` | Уровень логирования (`debug`, `info`, `warn`, `error`) |

### Пример

```bash
# Запуск с указанием директории репозиториев
iso2repo --dir /mnt/repos --port 4309 --level debug
```

После запуска список репозиторив становится доступен в WEB-интерфейсе:

```
http://<host>:4309
```

Для подключения к репозиторию из APT (в `/etc/apt/sources.list`):

```
deb [arch=amd64] http://<host>:4309/repo/<имя>.iso <distribute> <components>
```

Для пользовательского репозитория (custom):

```
deb [arch=amd64 trusted=yes] http://<host>:4309/repo/<имя>.iso custom contrib main non-free
```

### Сборка

```bash
# Сборка под текущую ОС
task build

# Сборка под Windows
task build_windows

# Сборка под Linux
task build_linux
```

## Как это работает

1. Программа периодически сканирует корневую директорию и сравнивает текущее состояние файлов с предыдущим.
2. При обнаружении нового файла или директории с расширением `.iso` определяется его тип:
   - Если это `.iso` файл — проверяется, является ли он APT-репозиторием.
   - Если это директория с расширением `.iso` — проверяется, содержит ли она структуру APT-репозитория (`dists/`, `Release`). Если нет — она обрабатывается как пользовательский репозиторий с `.deb` файлами.
3. При добавлении или удалении файлов внутри пользовательского репозитория программа автоматически пересканирует его и обновляет сгенерированные `Packages` и `Release`.
4. HTTP-сервер принимает события о найденных/потерянных репозиториях и обслуживает запросы APT-клиентов.
5. Все остальные файлы и директории из корневого каталога (не являющиеся репозиториями) доступны по адресу `/static/` для просмотра в браузере — PDF, TXT, видео, аудио и любые другие форматы.

## Зависимости

### 7z (для работы с запакованными ISO-образами)

Для работы с запакованными ISO-образами (файлами `.iso`) программа использует архиватор `7z`. Если в системе не установлен `7z`, программа не сможет распаковать ISO-образ для анализа его содержимого.

**Установка на Windows:**

Скачайте и установите [7-Zip](https://www.7-zip.org/) официального сайта. Убедитесь, что путь к `7z.exe` (обычно `C:\Program Files\7-Zip\`) добавлен в системную переменную `PATH`, либо программа сама найдёт его в стандартном пути установки.

**Установка на Linux (Debian/Ubuntu):**

```bash
sudo apt install p7zip-full
```

**Установка на Linux (Arch Linux):**

```bash
sudo pacman -S p7zip
```

**Установка на Linux (Fedora/RHEL):**

```bash
sudo dnf install p7zip p7zip-plugins
```

## Подготовка репозиториев

### Скачивание APT-репозитория из интернета с помощью apt-mirror

Для создания локальной копии APT-репозитория из интернета можно использовать утилиту `apt-mirror`.

**Установка apt-mirror:**

```bash
sudo apt install apt-mirror
```

**Настройка и запуск:**

1. Отредактируйте файл конфигурации `/etc/apt/mirror.list`, указав нужный репозиторий. Например, для зеркала Ubuntu Oracular:

```
deb http://ru.archive.ubuntu.com/ubuntu oracular main restricted universe multiverse
deb http://ru.archive.ubuntu.com/ubuntu oracular-updates main restricted universe multiverse
deb http://ru.archive.ubuntu.com/ubuntu oracular-security main restricted universe multiverse
```

2. Запустите загрузку:

```bash
sudo apt-mirror
```

Все загруженные пакеты будут сохранены в директории, указанной в конфигурации (по умолчанию `/mnt/data/apt-mirror/mirror/ru.archive.ubuntu.com/ubuntu/`).

### Создание ISO-образа из скачанного репозитория

После того как репозиторий скачан, из него можно создать ISO-образ с помощью утилиты `genisoimage` (пакет `genisoimage` в Ubuntu/Debian):

```bash
sudo apt install genisoimage
```

Пример создания ISO-образа:

```bash
genisoimage -f -J -joliet-long -r -allow-lowercase -allow-multidot -allow-limited-size -o ~/repository-ru.archive.ubuntu.com-oracular.iso /mnt/data/apt-mirror/mirror/ru.archive.ubuntu.com/ubuntu/
```

**Пояснение параметров:**

| Параметр | Описание |
|----------|----------|
| `-f` | Следовать символическим ссылкам |
| `-J` | Создать Joliet-расширения для совместимости с Windows |
| `-joliet-long` | Разрешить длинные имена файлов в Joliet (до 103 символов) |
| `-r` | Использовать Rock Ridge-расширения с POSIX-правами |
| `-allow-lowercase` | Разрешить строчные буквы в именах файлов |
| `-allow-multidot` | Разрешить множественные точки в именах файлов |
| `-allow-limited-size` | Разрешить файлы размером более 2 ГБ |
| `-o` | Путь к выходному ISO-образу |

Готовый ISO-образ можно поместить в директорию, отслеживаемую `iso2repo`, и он сразу станет доступен как APT-репозиторий по HTTP.

## Лицензия

MIT License

Copyright © Стерликов Кирилл @2023-2026

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.