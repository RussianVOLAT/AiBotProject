# crypto-rates-service

Сервис курсов криптовалют: сбор курсов BTC/ETH по расписанию, REST API,
Telegram-бот, и слой NL-ответов/алертов поверх локальной LLM.

Финальный проект курса по Go-бэкенду, с сознательно применяемым workflow
AI-assisted разработки (см. `docs/DECISIONS.md`, запись про workflow).

## Стек
- Go
- PostgreSQL
- Ollama (локальная LLM, ai-gateway модуль)
- Docker / Docker Compose
- GitHub Actions (CI/CD)
- OpenAPI (swaggo)

## Статус
🚧 В разработке. Архитектура и план — см. `docs/ARCHITECTURE.md` и `docs/PROGRESS.md`.

## Запуск (появится по мере готовности)
```
docker compose up -d
```

## Документация
- [Архитектура](docs/ARCHITECTURE.md)
- [Журнал решений (ADR)](docs/DECISIONS.md)
- [Прогресс](docs/PROGRESS.md)
