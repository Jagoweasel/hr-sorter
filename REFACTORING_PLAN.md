# Refactoring Plan - HR Sorter

This plan outlines the steps to improve the architecture, maintainability, and reliability of the HR Sorter project.

## 1. High Severity: Architectural & Reliability (Status: COMPLETED)
*Goal: Decouple business logic from HTTP handlers and eliminate global state.*

### A. Repository Layer
- Created `internal/repository` package.
- Moved all raw SQL queries from web handlers to repository structs.
- Fixed N+1 query issues (e.g., fetching integrations for each account).
- Implemented `AccountRepository`, `IntegrationRepository`, `ContactRepository`, `SequenceRepository`, `MessageRepository`, and `FilterRepository`.

### B. Service Layer
- Created `internal/service` package.
- Moved complex logic (e.g., starting/stopping workers, auth flow coordination, sequence creation) from handlers to services.
- Ensured atomicity by wrapping multi-step database operations in transactions.

### C. Dependency Injection
- Replaced global `database.DB`, `tgManager`, and `hhManager` with struct fields.
- Refactored `web.Handler` struct to receive all dependencies (Repositories, Services, Templates) at initialization.

## 2. Medium Severity: Maintenance & Scalability (Status: COMPLETED)
*Goal: Improve code organization and UI performance.*

### A. Monolithic Handler Splitting
- Split the 1400-line `handlers.go` into functional files:
    - `account_handler.go`
    - `integration_handler.go`
    - `pipeline_handler.go`
    - `contact_handler.go`
    - `message_handler.go`
    - `filter_handler.go`

### B. Template Optimization
- Implemented `TemplateManager` to parse and cache all HTML files on startup.
- Moved all inline HTML generation (`fmt.Fprintf`) into `.html` templates using HTMX partials/fragments.
- Standardized HTMX communication patterns using `HX-Trigger` and `HX-Location`.

## 3. Low Severity: Cleanliness & Standards (Status: COMPLETED)
*Goal: Idiomatic Go and configuration management.*

### A. Schema Management
- Moved the SQL schema from `internal/database/db.go` to `internal/database/schema.sql`.
- Used `//go:embed` to include the schema in the binary.

### B. Constants & Configuration
- Extracted hardcoded values and improved logic for stage hierarchies and status synchronization.
