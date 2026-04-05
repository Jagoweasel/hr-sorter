# Требования к модулю авторизации HH (Go)

## 1. Функциональные требования
1. **Эмуляция браузера**: Использовать библиотеку для автоматизации браузера (рекомендуется `github.com/go-rod/rod` или `github.com/playwright-community/playwright-go`).
2. **Перехват кастомных схем**: Реализовать механизм `Request Interception` для отлова URL, начинающихся с `hhandroid://`.
3. **Обмен токенов**: Реализовать HTTP-клиент для взаимодействия с OAuth-эндпоинтами HH.
4. **Хранение сессии**: Реализовать интерфейс для сохранения/загрузки токенов и cookies (в JSON или БД).
5. **Авто-обновление**: Реализовать механизм `Refresh Token` при получении ошибок авторизации в API.

## 2. Технические детали
1. **User-Agent Generator**: Портировать логику генерации UA из Python. Формат: `ru.hh.android/...`.
2. **Типизация**: Описать структуры данных для ответов HH (TokenResponse, ErrorResponse).
3. **HTTP Клиент**: Все запросы должны содержать заголовок `X-HH-App-Active: true`.
4. **Безопасность**: Скрывать секреты (Client Secret) в переменных окружения или зашифрованном конфиге.

## 3. Обработка ошибок
1. **Капча**: При появлении капчи в браузере (Selector: `.g-recaptcha` или аналоги), процесс должен останавливаться для ручного решения (в non-headless моде) или выдавать ошибку.
2. **2FA**: Поддержка ввода кода из SMS/Email через консольный ввод (Stdin).
3. **Таймауты**: Установить таймаут на ожидание редиректа с кодом (не менее 60 секунд).

## 4. Пример структуры (Go)
```go
type HHAuthSession struct {
    AccessToken  string    `json:"access_token"`
    RefreshToken string    `json:"refresh_token"`
    ExpiresAt    time.Time `json:"expires_at"`
}

type Authenticator interface {
    Authorize(ctx context.Context, username string) (*HHAuthSession, error)
    RefreshToken(ctx context.Context, session *HHAuthSession) error
}
```
