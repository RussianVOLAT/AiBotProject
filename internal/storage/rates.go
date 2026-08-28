package storage

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// InsertRate сохраняет свежий курс. Вызывается из collector раз в 5 мин.
func (s *Storage) InsertRate(ctx context.Context, currency string, price float64) error {
	const q = `
		INSERT INTO rates (currency, price_usd)
		VALUES ($1, $2)
	`
	if _, err := s.pool.Exec(ctx, q, currency, price); err != nil {
		return fmt.Errorf("storage: insert rate %s: %w", currency, err)
	}
	return nil
}

// GetLatestRates возвращает последний известный курс по каждой отслеживаемой валюте.
// DISTINCT ON (currency) — способ Postgres получить "по одной строке на группу"
func (s *Storage) GetLatestRates(ctx context.Context) ([]Rate, error) {
	const q = `
		SELECT DISTINCT ON (currency) id, currency, price_usd, fetched_at
		FROM rates
		ORDER BY currency, fetched_at DESC
	`
	rows, err := s.pool.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("storage: get latest rates: %w", err)
	}
	defer rows.Close()

	var result []Rate
	for rows.Next() {
		var r Rate
		if err := rows.Scan(&r.ID, &r.Currency, &r.PriceUSD, &r.FetchedAt); err != nil {
			return nil, fmt.Errorf("storage: scan rate: %w", err)
		}
		result = append(result, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("storage: iterate rates: %w", err)
	}

	return result, nil
}

// GetRateStats возвращает текущий курс + min/max за 24ч + %-изменение за час.
func (s *Storage) GetRateStats(ctx context.Context, currency string) (*RateStats, error) {
	latest, err := s.getLatest(ctx, currency)
	if err != nil {
		return nil, err
	}

	minUSD, maxUSD, err := s.getMinMax24h(ctx, currency)
	if err != nil {
		return nil, err
	}

	changePercent, err := s.getChangePercent1h(ctx, currency, latest)
	if err != nil {
		return nil, err
	}

	price, err := latest.Float64()
	if err != nil {
		return nil, fmt.Errorf("storage: convert price: %w", err)
	}

	return &RateStats{
		Currency:        currency,
		PriceUSD:        price,
		MinUSD24h:       minUSD,
		MaxUSD24h:       maxUSD,
		FetchedAt:       latest.FetchedAt,
		ChangePercent1h: changePercent,
	}, nil
}

func (s *Storage) getLatest(ctx context.Context, currency string) (Rate, error) {
	const q = `
		SELECT id, currency, price_usd, fetched_at
		FROM rates
		WHERE currency = $1
		ORDER BY fetched_at DESC
		LIMIT 1
	`
	var r Rate
	err := s.pool.QueryRow(ctx, q, currency).Scan(&r.ID, &r.Currency, &r.PriceUSD, &r.FetchedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Rate{}, ErrNotFound
	}
	if err != nil {
		return Rate{}, fmt.Errorf("storage: get latest %s: %w", currency, err)
	}
	return r, nil
}

func (s *Storage) getMinMax24h(ctx context.Context, currency string) (min, max float64, err error) {
	const q = `
		SELECT
			COALESCE(MIN(price_usd), 0),
			COALESCE(MAX(price_usd), 0)
		FROM rates
		WHERE currency = $1 AND fetched_at > now() - interval '24 hours'
	`
	if err := s.pool.QueryRow(ctx, q, currency).Scan(&min, &max); err != nil {
		return 0, 0, fmt.Errorf("storage: get min/max %s: %w", currency, err)
	}
	return min, max, nil
}

// getChangePercent1h ищет ближайшую запись, сделанную час (или больше) назад,
// и считает %-изменение относительно текущей цены. Если такой записи нет
// (сервис работает меньше часа) — возвращает nil, а не ошибку: это ожидаемая
// ситуация, а не сбой.
func (s *Storage) getChangePercent1h(ctx context.Context, currency string, latest Rate) (*float64, error) {
	const q = `
		SELECT price_usd
		FROM rates
		WHERE currency = $1 AND fetched_at <= now() - interval '1 hour'
		ORDER BY fetched_at DESC
		LIMIT 1
	`
	var oldPrice float64
	err := s.pool.QueryRow(ctx, q, currency).Scan(&oldPrice)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("storage: get price 1h ago %s: %w", currency, err)
	}

	newPrice, err := latest.Float64()
	if err != nil {
		return nil, fmt.Errorf("storage: convert latest price: %w", err)
	}

	change := calcPercentChange(oldPrice, newPrice)
	return &change, nil
}

func calcPercentChange(oldPrice, newPrice float64) float64 {
	if oldPrice == 0 {
		return 0 // защита от деления на 0 (например, "бесплатная" тестовая запись)
	}
	return (newPrice - oldPrice) / oldPrice * 100
}
