package api

import "time"

// rateResponse  то, что реально уходит в JSON по GET /rates.
// Специально отдельно от storage.Rate: наружу не должен течь pgtype.Numeric
// или другие детали конкретной БД  контракт API должен быть стабилен,
// даже если завтра под storage сменится драйвер или схема.
type rateResponse struct {
	Currency  string    `json:"currency"`
	PriceUSD  float64   `json:"price_usd"`
	FetchedAt time.Time `json:"fetched_at"`
}

// rateStatsResponse  ответ по GET /rates/{currency}.
type rateStatsResponse struct {
	Currency  string    `json:"currency"`
	PriceUSD  float64   `json:"price_usd"`
	MinUSD24h float64   `json:"min_usd_24h"`
	MaxUSD24h float64   `json:"max_usd_24h"`
	FetchedAt time.Time `json:"fetched_at"`
	// Указатель, чтобы при отсутствии данных (сервис работает < часа)
	// в JSON ушёл честный null, а не подозрительный 0  то же соображение,
	// что и в storage.RateStats.ChangePercent1h.
	ChangePercent1h *float64 `json:"change_percent_1h"`
}

// errorResponse единый формат ошибок, чтобы клиент API не гадал,
// в каком виде прилетит текст ошибки.
type errorResponse struct {
	Error string `json:"error"`
}
