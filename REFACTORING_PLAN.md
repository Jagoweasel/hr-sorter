# Refactoring Plan - HR Sorter

This plan outlines the steps to improve the architecture, maintainability, and reliability of the HR Sorter project.

## 1. High Severity: Architectural & Reliability (Status: IN PROGRESS)
*Goal: Decouple business logic from HTTP handlers and eliminate global state.*

### A. Repository Layer
- Create `internal/repository` package.
- Move all raw SQL queries from `internal/web/handlers.go` to repository structs.
- Fix N+1 query issues (e.g., fetching integrations for each account).
- Implement `AccountRepository`, `IntegrationRepository`, `ContactRepository`, `SequenceRepository`.

### B. Service Layer
- Create `internal/service` package.
- Move complex logic (e.g., starting/stopping workers, auth flow coordination) from handlers to services.
- Ensure atomicity by wrapping multi-step database operations in transactions.

### C. Dependency Injection
- Replace global `database.DB`, `tgManager`, and `hhManager` with struct fields.
- Refactor `web.RegisterRoutes` to use a `Server` or `Handler` struct that receives dependencies at initialization.

## 2. Medium Severity: Maintenance & Scalability (Status: PENDING)
*Goal: Improve code organization and UI performance.*

### A. Monolithic Handler Splitting
- Split `internal/web/handlers.go` into:
    - `accounts.go`
    - `integrations.go`
    - `pipeline.go`
    - `contacts.go`
    - `filters.go`

### B. Template Optimization
- Implement a `TemplateCache` to avoid parsing HTML files on every request.
- Move all inline HTML generation (`fmt.Fprintf`) into `.html` templates using HTMX partials.

## 3. Low Severity: Cleanliness & Standards (Status: PENDING)
*Goal: Idiomatic Go and configuration management.*

### A. Schema Management
- Move the SQL schema from `internal/database/db.go` to a `schema.sql` file.

### B. Constants & Configuration
- Extract hardcoded magic strings (e.g., vacancy names, CSS classes) into constants or config files.
