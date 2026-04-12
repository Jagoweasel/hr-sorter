# HR Sorter v2.0

Приложение для мониторинга сообщений от рекрутеров в Telegram и HeadHunter с единым веб-интерфейсом.

## Возможности
- Поддержка нескольких аккаунтов Telegram (через Userbot).
- Поддержка нескольких аккаунтов HeadHunter (через эмуляцию Android приложения).
- Автоматический сбор контактов и истории переписки.
- Веб-интерфейс на HTMX и Tailwind CSS.
- Генерация PDF отчетов и воронка найма.

## Установка и запуск
### Быстрый старт через Docker (Рекомендуется для Alpha-версии)
Для запуска одной командой без установки Go и браузеров на хост-машину:
1. Создайте `.env` файл из `.env.example` и заполните API ключи.
2. Запустите приложение:
   ```bash
   docker compose up --build
   ```
3. Веб-интерфейс будет доступен по адресу: `http://localhost:3000`
4. Данные будут сохраняться в локальной папке `./data`.

### Локальная установка (Manual)
- **Go 1.25.1+** (рекомендуется)
- **Playwright** (требуется для работы с HeadHunter)

### 2. Настройка окружения
Скопируйте `.env.example` в `.env` и заполните:
```env
API_ID=... # Получить на my.telegram.org
API_HASH=...
HH_CLIENT_ID=HIOMIAS39CA9DICTA7JIO64LQKQJF5AGIK74G9ITJKLNEDAOH5FHS5G1JI7FOEGD
HH_CLIENT_SECRET=V9M870DE342BGHFRUJ5FTCGCUA1482AN0DI8C5TFI9ULMA89H10N60NOP8I4JMVS
HEADLESS=false # Установите false, чтобы видеть браузер при авторизации HH
```

### 3. Установка зависимостей
```bash
# 1. Установка браузеров Playwright (ОБЯЗАТЕЛЬНО)
go run github.com/playwright-community/playwright-go/cmd/playwright install --with-deps

# 2. Установка Go модулей
go mod download
```

### 4. Запуск
```bash
# Запуск сервера
go run ./cmd/hrsorter/main.go

# Запуск с расширенной отладкой HeadHunter
go run ./cmd/hrsorter/main.go --debug-hh --debug-trace
```

## Отладка и логи
Приложение поддерживает гибкую систему логов через флаги:
- `--debug-hh`: Основные события HeadHunter (авторизация, синхронизация).
- `--debug-trace`: Детальные действия бота и Playwright.
- `--debug-hh-net`: Сетевой шум (метрики, статика). Полезно только при глубоком дебаге.
- `--debug-tg`: Логи Telegram клиента.
- `--debug-all`: Включить всё.

В Web UI (раздел Logs) можно фильтровать логи по категориям, включая новый уровень **TRACE**.

## Технологический стек
- **Backend:** Go, `playwright-go`, `gotd/td`.
- **Database:** SQLite.
- **Frontend:** HTMX, Tailwind CSS, Go Templates.
