# Design Document: HR-SORTER Evolution

## 1. Introduction
HR-SORTER is a tool for managing recruitment processes across Telegram and HeadHunter. This document outlines the technical requirements for improving existing modules and implementing new features to reach a production-ready state.

## 2. Core Improvements

### 2.1. HeadHunter Registration (OTP/SMS Flow)
**Objective:** Replace the current manual OAuth code extraction with an automated browser-based flow.
- **Requirements:**
    - Integrate `playwright-go` (or similar) to handle the HH login form in the background.
    - Implement a state machine in `hhclient` to handle:
        1. Identification (Email/Phone submission).
        2. OTP entry (Waiting for user input via UI).
        3. Captcha handling (Streaming image to UI if necessary).
        4. Token exchange upon successful redirect to `hhandroid://`.
- **UI:** A multi-step modal: `Phone/Email` -> `Captcha (if needed)` -> `OTP Code`.

### 2.2. PDF Reporting Engine
**Objective:** Fix encoding issues, layout breakage, and add professional branding.
- **Requirements:**
    - **Cyrillic Support:** Embed TTF/OTF fonts (e.g., Inter or DejaVu) into the binary using `go:embed` to ensure consistent rendering across environments.
    - **Layout Stability:** Refactor `maroto` logic to use flexible grid systems instead of absolute positioning where possible. Prevent text overflow in tables.
    - **Branding:** Add custom headers (Project Name, Generation Date) and footers (Page X of Y).
    - **Content:** Include more detailed KPIs and visual "recruitment funnel" charts.

### 2.3. Database & Performance
**Objective:** Improve stability and response times under load.
- **Requirements:**
    - **Optimization:** Audit all SQL queries and add missing indexes on `messages(integration_id, timestamp)` and `sequences(account_id, status)`.
    - **WAL Mode:** Ensure SQLite `WAL` mode and `busy_timeout` are correctly tuned for concurrent reads/writes.
    - **Caching:** Implement an LRU cache (e.g., `golang-lru`) for static or slow-changing data:
        - Message filters.
        - Integration settings.
        - Frequently accessed sequence metadata.

## 3. New Features

### 3.1. HH Funnel: Application Counting
**Objective:** Track the number of applications sent from a specific resume to calculate full funnel conversion.
- **Requirements:**
    - Fetch `/negotiations` data from HH API.
    - Map applications to existing `sequences` or create "ghost" sequences for initial tracking.
    - Store application count per resume/vacancy in the database.

### 3.2. Localization (i18n)
**Objective:** Provide a multi-language interface (English/Russian).
- **Requirements:**
    - Use JSON translation files (`en.json`, `ru.json`).
    - Implement a template helper function `{{ tr "key" }}` for Go templates.
    - Store language preference in Cookies or User Profile.

### 3.3. Dark Mode
**Objective:** Add a modern UI theme toggle.
- **Requirements:**
    - Utilize Tailwind CSS `dark` class strategy.
    - Implement a theme switcher in the navigation bar.
    - Persist choice in `localStorage`.

### 3.4. System Logs in UI
**Objective:** Allow administrators to debug integrations without terminal access.
- **Requirements:**
    - Create a log-streaming service using WebSockets or SSE (Server-Sent Events).
    - Attach a custom Hook to the `zap` logger to broadcast logs to the UI.
    - Add a "Logs" page or slide-over panel with level filtering (INFO/DEBUG/ERROR).

### 3.5. Vacancy Typing & Mapping
**Objective:** Categorize job titles for better reporting (e.g., "Go Developer" vs "Lead Backend").
- **Requirements:**
    - Add a `category` field (enum: Developer, Lead, etc.) to the `sequences` table.
    - Implement a `vacancy_mappings` table:
        - `pattern` (Regex): e.g., `.*Lead.*`.
        - `category`: `Lead`.
    - Automatically categorize new sequences based on these rules.
    - Provide a UI for users to manage mapping rules and manually override categories.

## 4. Technical Stack Considerations
- **Frontend:** HTMX + Tailwind CSS (Stay consistent with the current lightweight approach).
- **Backend:** Go 1.21+ (utilizing `go:embed` for assets).
- **Libraries:**
    - `maroto/v2` (for improved PDF).
    - `playwright-go` (for HH auth).
    - `nksm/go-i18n` (or simple custom implementation for translations).
