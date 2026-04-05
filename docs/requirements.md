# Product Requirements Document (PRD): HR-SORTER v2.0

## 1. Project Overview
HR-SORTER is a specialized tool for developers and recruiters to manage recruitment pipelines across Telegram (via MTProto) and HeadHunter (via API). This document specifies the requirements for evolving the project from a prototype to a production-ready application.

## 2. Technical Requirements

### 2.1. HeadHunter Integration & Authentication
- **Requirement:** Implement an automated OAuth flow using Playwright-go to handle SMS/Email OTP.
- **Details:**
    - Develop a backend state machine in `internal/hhclient` to manage the authentication lifecycle.
    - Support identification (phone/email), OTP entry, and captcha resolution.
    - Captcha images must be streamed to the web UI when requested by HH.
    - Tokens must be automatically exchanged and stored in the database upon successful redirect to `hhandroid://`.
- **UI:** A reactive multi-step modal in the "Accounts" section.

### 2.2. Reporting System (PDF/Excel)
- **Requirement:** Upgrade the PDF engine for professional-grade reporting.
- **Details:**
    - **Encoding:** Embed `Inter` or `DejaVuSans` fonts into the binary via `go:embed` for full Cyrillic support.
    - **Layout:** Use `maroto/v2` with a grid-based layout to ensure no text overflows or layout breakages on any screen size.
    - **Branding:** Include a standard header (HR-SORTER Logo, Report Date, Account Name) and footer (Page X of Y).
    - **Analytics:** Add KPI blocks (Response Rate, Hire Rate) and a visual recruitment funnel (Total -> Screening -> Tech -> Offer -> Accepted).

### 2.3. Performance & Stability
- **Requirement:** Optimize the database layer and introduce caching.
- **Details:**
    - **Indexing:** Add composite indexes on `messages(integration_id, timestamp)` and `sequences(account_id, status)`.
    - **SQLite Tuning:** Configure `PRAGMA` for WAL mode, `busy_timeout=5000`, and `synchronous=NORMAL`.
    - **Caching:** Implement `golang-lru` to cache message filters, account settings, and integration states to reduce disk I/O.

### 3. Functional Features

### 3.1. HH Application Analytics
- **Requirement:** Track the number of outgoing applications from HH resumes.
- **Details:**
    - Fetch data from HH `/negotiations` endpoint.
    - Store application counts in the database to calculate top-of-funnel conversion rates.

### 3.2. Internationalization (i18n)
- **Requirement:** Support English and Russian languages.
- **Details:**
    - Store translations in `internal/i18n/*.json`.
    - Implement a `{{ tr "key" }}` function for Go templates.
    - Default to browser language, allow manual toggle in UI.

### 3.3. Dark Mode
- **Requirement:** Add a "Dark" theme for the web UI.
- **Details:**
    - Use Tailwind CSS `dark:` variant classes.
    - Add a persistent toggle (stored in `localStorage`).

### 3.4. Real-time System Logs
- **Requirement:** Display backend logs in the web interface for debugging.
- **Details:**
    - Stream `zap` logger output via WebSocket or SSE (Server-Sent Events).
    - Add a dedicated "System Logs" panel with level filtering (Debug/Info/Error).

### 3.5. Vacancy Mapping & Categorization
- **Requirement:** Automatically classify vacancies into categories (e.g., Developer, Lead).
- **Details:**
    - Add `category` field to `sequences`.
    - Create a regex-based mapping table (e.g., `.*Lead.*` -> `Lead`).
    - Provide a UI for users to manage these mapping rules.

## 4. Documentation Requirements
- **Requirement:** Keep user and developer documentation up to date.
- **Details:**
    - Update `README.md` with instructions for the new HH Auth flow and Reporting features.
    - Create/Update a `USER_GUIDE.md` explaining how to configure vacancy mapping and reporting.

## 5. Development Policy
- **Requirement:** Strict branching and commit policy.
- **Details:**
    - All work must be performed and committed **ONLY** to the current active branch: `pre-release`.
    - No direct commits to `master` or other branches are allowed during this phase.
