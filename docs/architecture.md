# HR-SORTER v2.0 Architecture

## Overview
HR-SORTER v2.0 is designed using a Clean Architecture approach, following the Standard Go Project Layout. The system is split into independent layers to ensure testability and maintainability.

## Package Structure

### `internal/domain`
Contains core business entities, DTOs, and interface definitions (contracts). This layer has no dependencies on other internal packages.

### `internal/auth/hh`
Модуль для автоматизированной авторизации в HeadHunter (Android Spoofing).
- **Authenticator**: Управляет жизненным циклом сессий Playwright, поддерживая мультиаккаунтинг.
- **AuthFlow**: Инкапсулирует процесс логина, ожидание OTP/Captcha и перехват OAuth кода через Request Interception.
- **HHClient**: Высокоуровневая обертка для API HH с автоматическим обновлением токенов (Refresh Token) при 403 Forbidden.
- **SessionStorage**: Интерфейс для сохранения токенов и User-Agent в БД.

### `internal/hhclient`
Низкоуровневые HTTP-клиенты и утилиты для взаимодействия с API HH.

### `internal/report`
Generates PDF reports using `maroto/v2`.
- Uses `go:embed` for font assets (Inter/DejaVuSans).
- Implements a grid-based layout for professional KPI reporting.

### `internal/i18n`
Localization support for English and Russian.
- Uses `go:embed` to load JSON translation files.
- Provides a translation service for both backend logs and frontend templates.

### `internal/mapping`
Vacancy categorization engine.
- Uses regex-based rules to classify vacancies (e.g., "Developer", "Lead").
- Manageable via persistence layer.

### `internal/streaming`
Real-time system log streaming.
- Integrates with `zap` logger.
- Broadcasts logs via WebSocket or SSE to the web UI.

### `internal/storage`
Persistence layer using SQLite.
- Implements WAL mode and performance tuning.
- Includes an LRU cache layer for frequent lookups.

## Design Patterns
- **Repository Pattern**: Abstracting data access in `internal/domain`.
- **Strategy Pattern**: For different reporting types or notification channels.
- **State Pattern**: To manage the complex HH authentication flow.
- **Request Interception Pattern**: For capturing non-HTTP protocol (`hhandroid://`) redirects in Playwright.
- **Proxy Pattern**: HHClient wraps basic HTTP requests with token refresh logic and Android-specific headers.
- **Dependency Injection**: All dependencies are injected via interfaces to allow for easy mocking.
