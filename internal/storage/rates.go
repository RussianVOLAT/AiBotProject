package storage

import (
	"context"
	"errors"
	"fmt"

	"github.com/RussianVOLAT/AiBotProject/internal/domain"
	"github.com/jackc/pgx/v5"
)

// InsertRate сохраняет свежий курс. currency уже провалидированный
// domain.Currency, а не сырая строка: гарантия валидности приходит
// с вызывающей стороны (collector), storage не должен второй раз
// перепроверять то, что уже проверено на границе домена.
func (s *Storage) InsertRate(ctx context.Context, currency domain.Currency, price float64) error {
	const q = `
		INSERT INTO rates (currency, price_usd)
		VALUES ($1, $2)
	`
	if _, err := s.pool.Exec(ctx, q, currency.String(), price); err != nil {
		return fmt.Errorf("storage: insert rate %s: %w", currency, err)
	}
	return nil
}

// GetLatestRates возвращает последний известный курс по каждой отслеживаемой валюте.
func (s *Storage) GetLatestRates(ctx context.Context) ([]domain.Rate, error) {
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

	var result []domain.Rate
	for rows.Next() {
		var raw dbRate
		if err := rows.Scan(&raw.ID, &raw.Currency, &raw.PriceUSD, &raw.FetchedAt); err != nil {
			return nil, fmt.Errorf("storage: scan rate: %w", err)
		}
		r, err := raw.toDomain()
		if err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("storage: iterate rates: %w", err)
	}

	return result, nil
}

// GetRateStats возвращает текущий курс + min/max за 24ч + %-изменение за час.
func (s *Storage) GetRateStats(ctx context.Context, currency domain.Currency) (*domain.RateStats, error) {
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

	return &domain.RateStats{
		Currency:        currency,
		PriceUSD:        latest.PriceUSD,
		MinUSD24h:       minUSD,
		MaxUSD24h:       maxUSD,
		FetchedAt:       latest.FetchedAt,
		ChangePercent1h: changePercent,
	}, nil
}

func (s *Storage) getLatest(ctx context.Context, currency domain.Currency) (domain.Rate, error) {
	const q = `
		SELECT id, currency, price_usd, fetched_at
		FROM rates
		WHERE currency = $1
		ORDER BY fetched_at DESC
		LIMIT 1
	`
	var raw dbRate
	err := s.pool.QueryRow(ctx, q, currency.String()).Scan(&raw.ID, &raw.Currency, &raw.PriceUSD, &raw.FetchedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Rate{}, ErrNotFound
	}
	if err != nil {
		return domain.Rate{}, fmt.Errorf("storage: get latest %s: %w", currency, err)
	}
	return raw.toDomain()
}

func (s *Storage) getMinMax24h(ctx context.Context, currency domain.Currency) (min, max float64, err error) {
	const q = `
		SELECT
			COALESCE(MIN(price_usd), 0),
			COALESCE(MAX(price_usd), 0)
		FROM rates
		WHERE currency = $1 AND fetched_at > now() - interval '24 hours'
	`
	if err := s.pool.QueryRow(ctx, q, currency.String()).Scan(&min, &max); err != nil {
		return 0, 0, fmt.Errorf("storage: get min/max %s: %w", currency, err)
	}
	return min, max, nil
}

func (s *Storage) getChangePercent1h(ctx context.Context, currency domain.Currency, latest domain.Rate) (*float64, error) {
	const q = `
		SELECT price_usd
		FROM rates
		WHERE currency = $1 AND fetched_at <= now() - interval '1 hour'
		ORDER BY fetched_at DESC
		LIMIT 1
	`
	var oldPrice float64
	err := s.pool.QueryRow(ctx, q, currency.String()).Scan(&oldPrice)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("storage: get price 1h ago %s: %w", currency, err)
	}

	change := calcPercentChange(oldPrice, latest.PriceUSD)
	return &change, nil
}

// calcPercentChange чистая функция, покрыта юнит-тестом в rates_test.go.
func calcPercentChange(oldPrice, newPrice float64) float64 {
	if oldPrice == 0 {
		return 0
	}
	return (newPrice - oldPrice) / oldPrice * 100
}
