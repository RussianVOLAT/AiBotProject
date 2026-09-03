package domain

import "time"

// Rate курс валюты в конкретный момент времени. Единственная "истинная"
// модель курса в системе: не содержит pgtype.Numeric (деталь storage)
// и не содержит JSON-тегов (деталь api) это то, с чем работает бизнес-логика.
type Rate struct {
	Currency  Currency
	PriceUSD  float64
	FetchedAt time.Time
}

// RateStats агрегированная статистика по валюте: текущая цена + min/max
// за 24ч + %-изменение за час.
type RateStats struct {
	Currency  Currency
	PriceUSD  float64
	MinUSD24h float64
	MaxUSD24h float64
	FetchedAt time.Time
	// Указатель: nil значит "недостаточно истории для расчёта",
	// а не "изменения не было" (0.0 выглядело бы как реальный факт).
	ChangePercent1h *float64
}
