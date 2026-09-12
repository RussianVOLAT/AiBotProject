# Прогресс

## Готово
- [x] Согласована архитектура (монолит → выделение aigateway)
- [x] Согласован план на 2-3 недели, этапы 0-5 + буфер
- [x] Настроен workflow трёх аккаунтов (см. AI_WORKFLOW.md)
- [x] Стартовые docs/ и структура репозитория

## В работе (Этап 0 — инфраструктура)
- [x] Создать GitHub-репозиторий, сделать первый коммит
- [x] Настроить 3 Project'а в Claude по ролям, приложить docs/ как knowledge base
- [x] Скелет модулей: `cmd/`, `internal/{collector,api,bot,aigateway,storage}`
- [x] `docker-compose.yml` с Postgres

## В работе (Этап 1 — ядро)
- [x] Collector: опрос внешнего API раз в 5 мин для BTC/ETH → запись в Postgres
- [x] Таблицы `rates`, миграции
- [x] `GET /rates`
- [x] `GET /rates/{currency}` с min/max за день и % за час
- [x] Юнит-тесты на логику min/max/% изменения

## Готово (пост-ревью рефакторинг этапа 1)
- [x] `cmd/server/main.go`: единая точка входа, конструкторы вызываются в main,
      graceful shutdown по SIGINT/SIGTERM
- [x] `internal/domain`: доменные сущности `Currency`/`Rate`/`RateStats`,
      storage/collector/api переведены на них вместо разрозненных
      DTO/db-моделей
- [x] `internal/binance` вынесен из `collector` в отдельный пакет —
      источник цен и бизнес-логика сбора разделены, связь через интерфейс
- [x] Логирование: `log` → `slog`, структурированный JSON-вывод
- [x] `Dockerfile` (multi-stage) + сервис `app` в `docker-compose.yml`,
      весь стек поднимается одной командой `docker compose up`
- [x] Проверено end-to-end: контейнеры `postgres`+`app`, collector реально
      пишет курсы в БД, API их отдаёт

## Готово (Этап 2 — Telegram-бот, текстовые команды)
- [x] Миграция `subscriptions` (up/down), индекс под запрос шедулера
- [x] `internal/bot`: команды `/start`, `/rates`, `/rates {currency}`,
      `/start-auto {minutes}`, `/stop-auto`
- [x] Шедулер автопуша: sendMessage → MarkSent → best-effort deleteMessage,
      обработка 403 (деактивация подписки)
- [x] Wiring в `cmd/server/main.go`
- [x] Ручная проверка end-to-end через `docker compose up` + тестовый бот

## В работе / не начато
- [ ] Автопуш: полная статистика (min/max, % за час) вместо голой цены
- [ ] Inline-кнопки как UI поверх текстовых команд
- [ ] Тесты: юнит на `domain.Subscription.IsDue`, интеграционный на
      `storage.DueSubscriptions`
- [ ] Этап 3: aigateway (Ollama), NL-запросы, алерты по z-score
- [ ] Этап 4: вынос aigateway в отдельный контейнер
- [ ] Этап 5: CI/CD, OpenAPI, README, диаграмма архитектуры
- [ ] Буфер 2-3 дня
