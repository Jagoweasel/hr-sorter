# Build stage
FROM golang:1.23-bullseye AS builder

WORKDIR /app

# Install dependencies
COPY go.mod go.sum ./
RUN go mod download

# Install playwright-go CLI for browser installation
RUN go install github.com/playwright-community/playwright-go/cmd/playwright@v0.5700.1

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -o hr-sorter ./cmd/hrsorter/main.go

# Runtime stage
# We use a Debian-based image that is compatible with Playwright dependencies
FROM debian:bullseye-slim

WORKDIR /app

# Create data directory
RUN mkdir -p /app/data

# Install dependencies for Playwright and CA certificates
RUN apt-get update && apt-get install -y \
    ca-certificates \
    curl \
    gnupg \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/*

# Copy the binary and other files from the builder stage
COPY --from=builder /app/hr-sorter .
COPY --from=builder /app/templates ./templates
COPY --from=builder /go/bin/playwright /usr/local/bin/playwright

# Install Playwright browsers (chromium) and their system dependencies
# This command installs both the browser and the required system libraries
RUN playwright install chromium --with-deps

# Expose port
EXPOSE 3000

# Command to run the application
CMD ["./hr-sorter"]

