CREATE TABLE IF NOT EXISTS rates (
    id          BIGSERIAL PRIMARY KEY,
    currency    TEXT NOT NULL,
    price_usd   NUMERIC NOT NULL,
    fetched_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_rates_currency_fetched
    ON rates (currency, fetched_at DESC);