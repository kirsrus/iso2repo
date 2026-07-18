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