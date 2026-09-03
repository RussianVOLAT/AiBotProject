//go:build integration

package storage

import (
	"context"
	"testing"

	"github.com/RussianVOLAT/AiBotProject/internal/domain"
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

	testCurrency, err := domain.NewCurrency("TESTCOIN")
	if err != nil {
		t.Fatalf("build test currency: %v", err)
	}

	cleanup := func() {
		if _, err := st.pool.Exec(ctx, "DELETE FROM rates WHERE currency = $1", testCurrency.String()); err != nil {
			t.Logf("cleanup warning: %v", err)
		}
	}
	cleanup()
	defer cleanup()

	if err := st.InsertRate(ctx, testCurrency, 42.5); err != nil {
		t.Fatalf("insert rate: %v", err)
	}

	latest, err := st.GetLatestRates(ctx)
	if err != nil {
		t.Fatalf("get latest rates: %v", err)
	}

	var found bool
	for _, r := range latest {
		if r.Currency != testCurrency {
			continue
		}
		if r.PriceUSD != 42.5 {
			t.Errorf("got price %v, want 42.5", r.PriceUSD)
		}
		found = true
	}
	if !found {
		t.Error("test currency not found in GetLatestRates result")
	}

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
	if stats.ChangePercent1h != nil {
		t.Errorf("stats.ChangePercent1h = %v, want nil (no data 1h ago)", *stats.ChangePercent1h)
	}

	nonexistent, err := domain.NewCurrency("NOEXIST")
	if err != nil {
		t.Fatalf("build nonexistent currency: %v", err)
	}
	_, err = st.GetRateStats(ctx, nonexistent)
	if err == nil {
		t.Error("expected ErrNotFound for nonexistent currency, got nil")
	}
}
