# Build stage [cite: 1]
FROM golang:1.25 AS builder

WORKDIR /app

# Install dependencies
COPY go.mod go.sum ./
RUN go mod download

# Install playwright-go CLI for browser installation
RUN go install github.com/playwright-community/playwright-go/cmd/playwright@v0.5700.1

# Copy source code
COPY . .

# ВАЖНО: Если для SQLite используется mattn/go-sqlite3 (а не pure-go драйвер), 
# то CGO_ENABLED должен быть 1. Если проект успешно собирался раньше, оставляем 0.
RUN CGO_ENABLED=0 GOOS=linux go build -o hr-sorter ./cmd/hrsorter/main.go

# Runtime stage
# Меняем debian на ubuntu:jammy — это золотой стандарт для Playwright
FROM ubuntu:jammy

WORKDIR /app

# Create data directory
RUN mkdir -p /app/data

# Install basic utilities
RUN apt-get update && apt-get install -y \
    ca-certificates \
    curl \
    gnupg \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/*

# Copy the binary and other files from the builder stage
COPY --from=builder /app/hr-sorter .
COPY --from=builder /app/templates ./templates
COPY --from=builder /app/static ./static
COPY --from=builder /go/bin/playwright /usr/local/bin/playwright

# Жестко задаем путь установки браузеров (чтобы он не потерялся в ~/.cache)
ENV PLAYWRIGHT_BROWSERS_PATH=/ms-playwright

# Устанавливаем Chromium и ВСЕ системные зависимости
# DEBIAN_FRONTEND=noninteractive предотвращает зависание докера на вопросах о таймзонах
RUN DEBIAN_FRONTEND=noninteractive playwright install chromium --with-deps

# Expose port
EXPOSE 3000

# Command to run the application
CMD ["./hr-sorter"]