-- Подписки на автопуш курсов в Telegram.
-- user_id = chat_id пользователя в Telegram, он же PK: один юзер = одна подписка
-- (переподписка через /start-auto просто обновляет строку, см. ON CONFLICT
-- в internal/storage/subscriptions.go).
CREATE TABLE subscriptions (
    user_id           BIGINT PRIMARY KEY,
    interval_minutes  INT NOT NULL,
    active            BOOLEAN NOT NULL DEFAULT true,
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- last_sent_at NULL = ещё ни разу не отправляли (см. ADR про NULL-багфикс
    -- в условии "созревшей" подписки планировщик обязан явно проверять IS NULL).
    last_sent_at      TIMESTAMPTZ,
    -- id последнего отправленного сообщения — нужен шедулеру, чтобы удалить
    -- предыдущее уведомление после отправки нового (см. ADR про автопуш).
    last_message_id   INT
);

-- Шедулер бота на каждом тике сканирует активные подписки и сравнивает
-- last_sent_at с текущим временем индекс по (active, last_sent_at)
-- ускоряет именно этот запрос (WHERE active = true AND (last_sent_at IS NULL OR ...)).
CREATE INDEX idx_subscriptions_active_last_sent
    ON subscriptions (active, last_sent_at);
