package storage

import (
	"context"
	"testing"
)

const testDSN = "postgres://appuser:devpassword@localhost:5432/crypto_rates?sslmode=disable"

func TestStorageIntegration(t *testing.T) {
	if err := RunMigrations(testDSN); err != nil {
		t.Fatalf("run migrations: %v", err)
	}

	ctx := context.Background()

	st, err := New(ctx, testDSN)
	if err != nil {
		t.Fatalf("connect to storage: %v", err)
	}
	defer st.Close()

	const testCurrency = "TEEEEEEESTCOIN"

	cleanup := func() {
		if _, err := st.pool.Exec(ctx, "DELETE FROM rates WHERE currency = $1", testCurrency); err != nil {
			t.Logf("cleanup warning: %v", err)
		}
	}
	cleanup()
	defer cleanup()

	if err := st.InsertRate(ctx, testCurrency, 42.5); err != nil {
		t.Fatalf("insert rate: %v", err)
	}

	//  GetLatestRates — проверяем, что запись находится среди последних курсов.
	latest, err := st.GetLatestRates(ctx)
	if err != nil {
		t.Fatalf("get latest rates: %v", err)
	}

	var found bool
	for _, r := range latest {
		if r.Currency != testCurrency {
			continue
		}
		price, err := r.Float64()
		if err != nil {
			t.Fatalf("convert price: %v", err)
		}
		if price != 42.5 {
			t.Errorf("got price %v, want 42.5", price)
		}
		found = true
	}
	if !found {
		t.Error("test currency not found in GetLatestRates result")
	}

	//  GetRateStats — проверяем, что min/max корректно вернулись
	// (для одной записи min == max == сама цена).
	stats, err := st.GetRateStats(ctx, testCurrency)
	if err != nil {
		t.Fatalf("get rate stats: %v", err)
	}
	if stats.PriceUSD != 42.5 {
		t.Errorf("stats.PriceUSD = %v, want 42.5", stats.PriceUSD)
	}
	if stats.MinUSD24h != 42.5 || stats.MaxUSD24h != 42.5 {
		t.Errorf("stats min/max = %v/%v, want 42.5/42.5", stats.MinUSD24h, stats.MaxUSD24h)
	}
	// Записи час назад не было — изменение за час должно быть неизвестно (nil)
	if stats.ChangePercent1h != nil {
		t.Errorf("stats.ChangePercent1h = %v, want nil (no data 1h ago)", *stats.ChangePercent1h)
	}

	//  GetRateStats для несуществующей валюты — должна вернуться ErrNotFound.
	_, err = st.GetRateStats(ctx, "NONEXISTENT_COIN_XYZ")
	if err == nil {
		t.Error("expected ErrNotFound for nonexistent currency, got nil")
	}
}
