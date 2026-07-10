# prunejuice 🧹

[English](README.md) | [Русский](README.ru.md)

[![CI](https://github.com/GeorgeTyupin/prunejuice/actions/workflows/ci.yml/badge.svg)](https://github.com/GeorgeTyupin/prunejuice/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/GeorgeTyupin/prunejuice.svg)](https://pkg.go.dev/github.com/GeorgeTyupin/prunejuice)
[![Go Report Card](https://goreportcard.com/badge/github.com/GeorgeTyupin/prunejuice)](https://goreportcard.com/report/github.com/GeorgeTyupin/prunejuice)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

**Следит за диском сервера и автоматически освобождает место.**

`prunejuice` наблюдает за точкой монтирования и, когда занятое место превышает порог, запускает по очереди список команд очистки (vacuum journald, `apt clean`, `docker system prune`, ...), останавливаясь, как только диск опускается ниже порога. Если освободить место не удалось или что-то пошло не так — отправляет алерт в Telegram.

> Prune juice — это то, что пьют, чтобы прочистить организм. Это то же самое, но для диска.

Поставляется **в двух режимах одновременно**:

- 🛠️ **отдельный бинарник** для сервера (systemd timer или daemon), с алертами в Telegram из коробки;
- 📦 **Go-библиотека** для встраивания в свой сервис — без зависимости на Telegram и сеть, если не подключать.

---

## Почему это появилось

Проект родился из реального инцидента на машине `sof-tunnel`: journald и логи приложений тихо заняли весь корневой раздел до 100%, после чего Docker перестал запускать контейнеры, а SSH-сессии начали отваливаться. Потребовался человек в 3 ночи, чтобы выполнить три команды, которые скрипт мог бы выполнить сам — и, что важнее, сообщить об этом до того, как проблему заметят пользователи.

Полный post-mortem и ручной runbook, который этот инструмент автоматизирует, — в [`docs/server-disk-cleanup-runbook.md`](docs/server-disk-cleanup-runbook.md).

---

## Возможности

- ✅ Мониторинг диска по порогу через [`gopsutil`](https://github.com/shirou/gopsutil).
- ✅ Упорядоченные **настраиваемые** шаги очистки — включай/выключай/переставляй без пересборки.
- ✅ Проверяет место после каждого шага и **останавливается, как только хватит**.
- ✅ Алерты в Telegram при *"не удалось освободить место"* **и** при *"сама утилита упала"*.
- ✅ Таймаут на каждую команду; зависшие команды убиваются, а не висят вечно.
- ✅ Graceful shutdown по `SIGINT`/`SIGTERM`.
- ✅ Логи с ротацией — утилита, которая следит за диском, **никогда** не заполнит его сама.
- ✅ Деструктивные операции выключены по умолчанию (`docker prune` — **off**).
- ✅ Чистая архитектура, минимум зависимостей, юнит-тесты на логику принятия решений.

---

## Установка

```bash
# как CLI
go install github.com/GeorgeTyupin/prunejuice/cmd/prunejuice@latest

# как библиотека
go get github.com/GeorgeTyupin/prunejuice
```

Или собрать из исходников:

```bash
git clone https://github.com/GeorgeTyupin/prunejuice
cd prunejuice
make build      # бинарник окажется в ./bin/prunejuice
```

---

## Быстрый старт (CLI)

1. Сгенерировать конфиг:

   ```bash
   prunejuice -print-config > config.yaml
   ```

2. Отредактировать (см. [Конфигурация](#конфигурация)). Минимум — выставить порог и, если нужны алерты, включить Telegram.

3. Запустить один раз (рекомендуемый режим, через systemd timer):

   ```bash
   prunejuice -config /etc/prunejuice/config.yaml
   ```

   Или как долгоживущий daemon:

   ```bash
   prunejuice -config /etc/prunejuice/config.yaml -daemon
   ```

### Рекомендуемый деплой: systemd timer

One-shot через timer проще и надёжнее daemon: нет долгоживущего процесса, которому надо следить за утечками, расписание — в одном очевидном месте. Скопировать юниты из [`deploy/systemd`](deploy/systemd):

```bash
sudo cp bin/prunejuice /usr/local/bin/
sudo mkdir -p /etc/prunejuice && sudo cp config.yaml /etc/prunejuice/
sudo cp deploy/systemd/prunejuice.* /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now prunejuice.timer
```

Секреты можно передавать через `systemctl edit prunejuice.service` (environment override), а не класть в конфиг. **Подробнее о daemon vs. timer — в [`deploy/systemd/README.md`](deploy/systemd/README.md).**

---

## Запуск в Docker

prunejuice может работать как контейнер, который следит за хостом. Хитрость в том, что корень хоста монтируется read-only для проверки диска, а команды очистки выполняются в namespace хоста через `nsenter -t 1` — так `journalctl`, `apt` и `docker` работают с хостом, а не с контейнером.

```bash
# отредактируй configs/config.docker.yaml при необходимости
cat > prunejuice.env <<'EOF'
PRUNEJUICE_TELEGRAM_BOT_TOKEN=123456789:AA...
PRUNEJUICE_TELEGRAM_CHAT_ID=123456789
PRUNEJUICE_HOST=sof-tunnel
EOF
chmod 600 prunejuice.env

docker compose up -d --build
docker compose logs -f
```

[`docker-compose.yml`](docker-compose.yml) запускает контейнер с `pid: host` и `privileged: true` — это обязательно для `nsenter`.

> ⚠️ **Безопасность:** контейнер с `privileged` + `pid: host` фактически имеет root на хосте. Это цена обслуживания хоста из контейнера. Если Docker не нужен специально, деплой через [systemd timer](deploy/systemd) проще и менее привилегирован.

Конфиг для Docker — [`configs/config.docker.yaml`](configs/config.docker.yaml), каждый шаг там обёрнут в `nsenter`.

---

## Конфигурация

`prunejuice` читает YAML. Секреты можно передавать через переменные окружения:

| Переменная | Переопределяет |
| --- | --- |
| `PRUNEJUICE_TELEGRAM_BOT_TOKEN` | `telegram.bot_token` |
| `PRUNEJUICE_TELEGRAM_CHAT_ID` | `telegram.chat_id` |
| `PRUNEJUICE_HOST` | `host` |

Полный пример с комментариями — [`configs/config.example.yaml`](configs/config.example.yaml). Основное:

```yaml
mount_path: /            # какой диск следить
threshold_percent: 85    # запускать очистку при этом % занятости
check_interval: 5m       # интервал в режиме daemon (для one-shot игнорируется)
command_timeout: 60s     # максимальное время выполнения одной команды

log:
  level: info
  file: /var/log/prunejuice/prunejuice.log
  max_size_mb: 10        # ротация при 10 МБ, хранить 3 файла → не более ~40 МБ
  max_backups: 3

telegram:
  enabled: true          # выключено ⇒ только логи
  # токен и chat id — из переменных окружения выше

steps:
  - name: journal-vacuum
    command: journalctl
    args: ["--vacuum-time=7d"]
    enabled: true
  - name: apt-clean
    command: apt
    args: ["clean"]
    enabled: true
  - name: docker-prune
    command: docker
    args: ["system", "prune", "-f"]
    enabled: false        # ⚠️ деструктивно — включать намеренно
    requires_binary: docker  # пропускается автоматически, если docker не установлен
```

---

## Использование как библиотека

Встроить тот же движок в свой Go-сервис и запускать на своём расписании (из HTTP-хендлера, cron-горутины, k8s sidecar и т.д.).
**Telegram не тянется в runtime**, если не вызывать `WithTelegram`. По умолчанию алерты никуда не идут — маршрутизируй их сам.

```go
package main

import (
    "context"
    "log"

    "github.com/GeorgeTyupin/prunejuice"
)

func main() {
    p, err := prunejuice.New(prunejuice.Config{
        MountPath:        "/",
        ThresholdPercent: 85,
        Steps:            prunejuice.DefaultSteps(),
    })
    if err != nil {
        log.Fatal(err)
    }

    report, err := p.RunOnce(context.Background())
    if err != nil {
        log.Printf("prune failed: %v", err)
    }
    if !report.Resolved() {
        log.Printf("диск всё ещё занят на %.1f%% после очистки", report.FinalUsage.UsedPercent)
        // ... подключи своё алертирование / метрики
    }
}
```

Подключить Telegram, свой алерт-синк или фейки для тестов:

```go
p, _ := prunejuice.New(cfg,
    prunejuice.WithTelegram(token, chatID),   // алерты в Telegram
    prunejuice.WithLogNotifier(myLogger),     // дублировать алерты в slog
    prunejuice.WithNotifier(myPagerDuty),     // или любой Notifier
    prunejuice.WithLogger(myLogger),          // операционные логи по шагам
)
```

`Notifier` — интерфейс с одним методом, поэтому подключить Slack, PagerDuty, email или Prometheus pushgateway просто:

```go
type Notifier interface {
    Notify(ctx context.Context, alert prunejuice.Alert) error
}
```

Рабочий пример — [`examples/library`](examples/library).

---

## Архитектура

`prunejuice` следует принципам clean architecture — зависимости направлены **внутрь**, ядро ничего не знает о Telegram или gopsutil.

Модуль предоставляет ровно **один публичный пакет** (`prunejuice`, фасад); всё остальное живёт под `internal/`, чтобы реализация могла меняться без поломки пользователей библиотеки.

```
cmd/prunejuice              ← wiring: флаги, сигналы, конфиг → адаптеры → движок
      │
      ├── internal/config   ← загрузка YAML + env через cleanenv (только CLI)
      ├── internal/logging  ← slog + ротация по размеру (только CLI)
      │
prunejuice (facade)         ← единственный публичный пакет: New(), RunOnce(), Run(),
      │                        функциональные опции, реэкспортированные типы домена
      │
   internal/service         ← use case: решение по порогу, оркестрация шагов
      │                        (чистый; зависит только от domain — покрыт юнит-тестами)
      │
   internal/domain          ← сущности (DiskUsage, CleanupStep, Report, Alert) +
                              интерфейсы-порты (DiskProber, Runner, Notifier). Нет зависимостей.
      ▲
   internal/adapter/*       ← реализации портов: disk (gopsutil), command (os/exec),
                              telegram, notify (noop/log/multi)

configs/                    ← YAML-конфиги (config.example.yaml, config.docker.yaml)
```

Логика принятия решений живёт за интерфейсами, поэтому весь flow — "чистить или нет, что запускать, слать ли алерт" — тестируется in-memory фейками без реального диска, шелла и сети. См. [`internal/service/engine_test.go`](internal/service/engine_test.go).

---

## Безопасность

- **Ничего деструктивного по умолчанию.** Каждый шаг очистки по умолчанию `enabled: false`; `docker system prune -f` особенно — выключен намеренно, потому что может удалить ресурсы других стеков на хосте.
- **Таймауты везде.** Каждая команда выполняется под `command_timeout`; зависшая команда убивается.
- **Ограниченные логи.** Утилита, которая следит за переполнением диска, сама не заполнит его: файловые логи ротируются при `max_size_mb` и хранят не более `max_backups` файлов.
- **Минимум привилегий.** Нужны только чтение статистики диска и запуск включённых команд очистки. Пример hardened-юнита для systemd — в runbook.

---

## Разработка

```bash
make test        # go test ./... с race detector
make lint        # gofmt + go vet (+ golangci-lint если установлен)
make build       # собрать бинарник в ./bin
```

Contributions приветствуются — см. [CONTRIBUTING.md](CONTRIBUTING.md) и [Code of Conduct](CODE_OF_CONDUCT.md). Уязвимости: [SECURITY.md](SECURITY.md).

## Лицензия

[MIT](LICENSE) © George Tyupin
