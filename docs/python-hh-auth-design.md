# Дизайн системы авторизации HH (Android App Spoofing)

## Обзор
Авторизация имитирует поведение официального мобильного приложения HeadHunter для Android. Это позволяет использовать OAuth-флоу с `redirect_uri`, который легко перехватить программно, и получать токены с расширенными правами доступа.

## Основные константы
- **OAuth Base URL**: `https://hh.ru/oauth/`
- **API Base URL**: `https://api.hh.ru/`
- **Client ID**: `HIOMIAS39CA9DICTA7JIO64LQKQJF5AGIK74G9ITJKLNEDAOH5FHS5G1JI7FOEGD`
- **Client Secret**: `V9M870DE342BGHFRUJ5FTCGCUA1482AN0DI8C5TFI9ULMA89H10N60NOP8I4JMVS`
- **Redirect URI**: `hhandroid://oauth`
- **Custom Scheme**: `hhandroid`

## Заголовки (Headers)
Для всех запросов (и в браузере, и к API) необходимо использовать специфичный User-Agent:
- **User-Agent**: `ru.hh.android/7.<minor>.<patch>, Device: <model>, Android OS: <version> (UUID: <uuid4>)`
  *Пример*: `ru.hh.android/7.120.12345, Device: 23053RN02A, Android OS: 13 (UUID: 550e8400-e29b-41d4-a716-446655440000)`
- **X-HH-App-Active**: `true` (обязателен для API запросов)

## Этапы процесса (The Flow)

### Шаг 1: Инициализация браузера
Запускается headless-браузер (Playwright/Puppeteer) с эмуляцией Android-устройства.
- **Действие**: Подписка на событие запроса (`request`), чтобы перехватить редирект на кастомную схему.

### Шаг 2: Запрос на авторизацию (GET)
Переход по URL:
`https://hh.ru/oauth/authorize?client_id=[CLIENT_ID]&redirect_uri=hhandroid://oauth&response_type=code`

**Процесс**:
1. Пользователь вводит логин/пароль или OTP.
2. После успеха HH инициирует редирект на `hhandroid://oauth?code=AUTH_CODE`.
3. Браузер не может открыть этот URL, но скрипт перехватывает его из сетевого события.

### Шаг 3: Обмен кода на токен (POST)
**Endpoint**: `https://hh.ru/oauth/token`
**Content-Type**: `application/x-www-form-urlencoded`
**Тело запроса**:
- `grant_type`: `authorization_code`
- `client_id`: `[CLIENT_ID]`
- `client_secret`: `[CLIENT_SECRET]`
- `code`: `[AUTH_CODE]`

**Ответ**:
```json
{
  "access_token": "USER...",
  "refresh_token": "...",
  "expires_in": 1209600
}
```

### Шаг 4: Использование API
Для каждого запроса добавляется заголовок:
`Authorization: Bearer [access_token]`

### Шаг 5: Обновление токена (Refresh)
Если API возвращает `403 Forbidden`, выполняется POST на `/oauth/token`:
- `grant_type`: `refresh_token`
- `refresh_token`: `[stored_refresh_token]`
