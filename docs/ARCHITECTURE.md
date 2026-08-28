# Архитектура

## Текущая фаза
Модульный монолит. Микросервисы — не самоцель, а следующий шаг после того,
как границы модулей станут понятны на практике (см. DECISIONS.md).

## Модули (internal/)
| Модуль       | Ответственность                                                        |
|--------------|--------------------------------------------------------------------------|
| `collector`  | Опрос внешнего API курсов раз в 5 мин (BTC, ETH), запись в Postgres     |
| `storage`    | Слой доступа к БД (репозитории, миграции)                              |
| `api`        | REST API: `GET /rates`, `GET /rates/{currency}`, OpenAPI-спека         |
| `bot`        | Telegram-команды, подписки на автопуш, фоновый шедулер рассылки        |
| `aigateway`  | Обёртка над локальной LLM (Ollama): NL-ответы, объяснение алертов      |

## План выделения в отдельный сервис
`aigateway` — первый (и на срок 2-3 недели единственный) кандидат на вынос
в отдельный контейнер: у него принципиально другой профиль нагрузки
(инференс модели), и понятная причина для отдельного масштабирования.
Общение с основным монолитом — через REST (можно сменить на gRPC позже).

## Схема БД (черновик)
```sql
-- курсы
CREATE TABLE rates (
    id          BIGSERIAL PRIMARY KEY,
    currency    TEXT NOT NULL,        -- 'BTC', 'ETH'
    price_usd   NUMERIC NOT NULL,
    fetched_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_rates_currency_fetched ON rates (currency, fetched_at DESC);

-- подписки на автопуш в Telegram
CREATE TABLE subscriptions (
    user_id           BIGINT PRIMARY KEY,
    interval_minutes  INT NOT NULL,
    active            BOOLEAN NOT NULL DEFAULT true,
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_sent_at      TIMESTAMPTZ
);
```

## REST API
| Метод | Путь                     | Описание                                              |
|-------|--------------------------|--------------------------------------------------------|
| GET   | `/rates`                 | Текущие курсы всех отслеживаемых валют                 |
| GET   | `/rates/{currency}`      | Курс валюты + min/max за день + % изменения за час      |

## Telegram-команды
`/start`, `/rates`, `/rates {currency}`, `/start-auto {minutes}`, `/stop-auto`
— плюс NL-запросы свободным текстом через `aigateway` (этап 3).

## Принцип для aigateway
Любой ответ, сгенерированный LLM, строится поверх реальных цифр из БД
(RAG, не «из головы» модели) и обязан включать дисклеймер об отсутствии
финансовой рекомендации, если речь идёт о трендах/прогнозах.
