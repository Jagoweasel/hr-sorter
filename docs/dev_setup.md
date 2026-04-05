# Local Development Setup

This project uses Go and SQLite for the backend, with Playwright for HeadHunter authorization.

## Prerequisites
- Go 1.25.1+
- Docker and Docker Compose (optional, for running in container)
- [Optional] [Playwright dependencies](https://playwright.dev/docs/intro#installing-playwright) if running locally without Docker.

## Setting up Environment Variables
1. Copy `.env.example` to `.env`:
   ```bash
   cp .env.example .env
   ```
2. Fill in the required variables:
   - `API_ID`, `API_HASH`: Your Telegram API credentials.
   - `HH_CLIENT_ID`, `HH_CLIENT_SECRET`: Your HeadHunter API credentials.

## Running with Docker Compose
To run the entire stack in a container:
```bash
docker compose up -d
```
The application will be available at `http://localhost:8080`.
The database file `hr-sorter.db` will be persisted in the `./data` directory.

## Local Development (without Docker)
1. Install dependencies:
   ```bash
   go mod download
   ```
2. Install Playwright browsers:
   ```bash
   go run github.com/playwright-community/playwright-go/cmd/playwright install --with-deps
   ```
3. Run the application:
   ```bash
   go run ./cmd/hr-sorter/main.go
   ```

## Running Tests
To run tests for the HeadHunter auth module:
```bash
go test -v ./internal/auth/hh/...
```
