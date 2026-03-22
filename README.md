# HR Sorter

A simple Go application to monitor incoming Telegram messages from recruiters/HRs across multiple accounts and store them in a SQLite database with an HTMX-powered web UI.

## Features
- Multiple Telegram accounts support (MTProto Userbot).
- Automatic contact and message capture.
- Lightweight SQLite database.
- Fast and reactive UI using HTMX and Tailwind CSS.

## Prerequisites
- Go 1.21+
- Telegram `API_ID` and `API_HASH` from [my.telegram.org](https://my.telegram.org).

## Setup
1. Clone the repository.
2. Copy `.env.example` to `.env` and fill in your Telegram API credentials.
3. Run `go mod tidy` to install dependencies.
4. Run the application:
   ```bash
   go run cmd/hr-sorter/main.go
   ```

## Adding Accounts
Currently, adding accounts is planned via a CLI helper or an internal web form.
(Implementation in progress).

## Debugging & Logging
The application uses a categorized logging system that is disabled by default. You can enable specific debug logs using command-line flags:

```bash
# Enable Telegram synchronization logs
go run cmd/hr-sorter/main.go --debug-sync

# Enable sequence creation logs
go run cmd/hr-sorter/main.go --debug-add

# Enable sequence history and movement logs
go run cmd/hr-sorter/main.go --debug-history

# Enable everything
go run cmd/hr-sorter/main.go --debug-all
```

When `--debug-history` is enabled, the application will output a visual representation of recruitment chains:
`[HISTORY] Seq #13 (Google): Initial Contact -> HR Screening -> Tech Interview 1 -> [REJECTED]`

## Tech Stack
- **Backend:** Go, `gotd/td` (MTProto), `sqlx`.
- **Database:** SQLite (pure Go driver).
- **Frontend:** HTMX, Tailwind CSS, Go Templates.

## Termination

fuser -k 8080/tcp
