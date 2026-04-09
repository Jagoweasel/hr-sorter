# Спецификация локального запуска HH Sorter

Для удобного запуска и тестирования приложения без необходимости локальной установки Go и настройки драйверов SQLite, используется Docker и Docker Compose.

## 1. Структура проекта
Убедитесь, что в корне проекта созданы необходимые файлы и папки:

```text
.
├── cmd/
│   └── app/
│       └── main.go
├── data/   # Директория для хранения файла sqlite.db
├── .env.example    # Пример переменных окружения
├── docker-compose.yml
├── Dockerfile
└── README.md       # Инструкция для пользователей
```

## 2. Dockerfile

Создайте файл Dockerfile в корне проекта. Здесь используется многоэтапная сборка (multi-stage build) с включенным CGO для корректной работы драйвера SQLite.

### Этап 1: Сборка

```
FROM golang:1.22-alpine AS builder
```

#### Устанавливаем зависимости для CGO (необходимо для SQLite)

```
RUN apk add --no-cache gcc musl-dev
WORKDIR /app
```

#### Копируем модули для кэширования

```
COPY go.mod go.sum ./
RUN go mod download
```

#### Копируем исходный код

```
COPY . .
```

#### Собираем бинарник с включенным CGO

```
RUN CGO_ENABLED=1 GOOS=linux go build -ldflags="-s -w" -o /app/hh-aggregator ./cmd/app
```

### Этап 2: Финальный образ

```
FROM alpine:latest
```

#### tzdata для корректной работы со временем

```
RUN apk --no-cache add tzdata
WORKDIR /app
```

#### Копируем бинарник из билдера

```
COPY --from=builder /app/hh-aggregator .
```

#### Создаем папку для базы данных

```
RUN mkdir -p /app/data
```

#### Точка входа

```
CMD ["./hhsorter"]
```

## 3. docker-compose.yml

Создайте файл docker-compose.yml в корне проекта. Этот конфиг описывает сборку образа, проброс портов и монтирование volume для сохранения данных SQLite при перезапусках.

```
version: '3.8'

services:
  app:
    build: 
      context: .
      dockerfile: Dockerfile
    container_name: hh-aggregator-alpha
    restart: unless-stopped
    ports:
      - "8080:8080" # Измените порт, если необходимо
    volumes:
      - ./data:/app/data # Сохранение БД на хосте
    env_file:
      - .env
    environment:
      - DB_PATH=/app/data/sqlite.db
```

## 4. Инструкция для README.md

Скопируйте этот блок в ваш README.md, чтобы пользователи знали, как запустить проект.
HH Aggregator (Alpha)

Утилита для агрегации откликов и сообщений от HR на hh.ru.

### Требования

```
    Docker
    Docker Compose
```

### Быстрый старт

#### Склонируйте репозиторий:

    ```Bash
    git clone [https://github.com/yourusername/hh-aggregator.git](https://github.com/yourusername/hh-aggregator.git)
    cd hh-aggregator
    ```

#### Настройте переменные окружения:

    ```Bash
    cp .env.example .env
    ```

    Откройте файл .env и впишите ваш токен от HH и другие требуемые параметры.

#### Запустите приложение:
    
    ```Bash
    docker-compose up --build -d
    ```

#### Использование:

    Перейдите в браузере по адресу http://localhost:8080.

### Логи и остановка

    Посмотреть логи: docker-compose logs -f

    Остановить приложение: docker-compose down

    Важно: Файл базы данных (sqlite.db) сохраняется локально в директории ./data. При пересоздании контейнеров ваши данные не будут потеряны.