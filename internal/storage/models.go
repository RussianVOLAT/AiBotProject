package storage

import (
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// Rate — одна запись курса валюты в конкретный момент времени.
// PriceUSD хранится как pgtype.Numeric (а не float64), потому что
// это ровно тот тип, ради которого я выбрал pgx нативно (см. DECISIONS.md,
// 2026-08-27) — NUMERIC в Postgres не теряет точность, float64 её теряет.
type Rate struct {
	ID        int64
	Currency  string
	PriceUSD  pgtype.Numeric
	FetchedAt time.Time
}

// Float64 конвертирует NUMERIC в float64.
// Я сознательно теряю часть точности здесь: для отображения курса
// пользователю (или для расчёта %-изменения) это ок, а вот если бы
// сервис проводил реальные финансовые операции, годился бы только
// shopspring/decimal или сама pgtype.Numeric без конвертации.
func (r Rate) Float64() (float64, error) {
	f, err := r.PriceUSD.Float64Value()
	if err != nil {
		return 0, err
	}
	return f.Float64, nil
}

// RateStats — агрегированная статистика по валюте для GET /rates/{currency}.
type RateStats struct {
	Currency  string
	PriceUSD  float64
	MinUSD24h float64
	MaxUSD24h float64
	FetchedAt time.Time

	// ChangePercent1h — указатель, а не float64: изменение за час может
	// быть попросту неизвестно.
	ChangePercent1h *float64
}
