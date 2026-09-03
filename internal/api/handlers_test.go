package api

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/RussianVOLAT/AiBotProject/internal/domain"
	"github.com/RussianVOLAT/AiBotProject/internal/storage"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

type fakeStore struct {
	rates     []domain.Rate
	stats     *domain.RateStats
	statsErr  error
	latestErr error
}

func (f *fakeStore) GetLatestRates(ctx context.Context) ([]domain.Rate, error) {
	return f.rates, f.latestErr
}

func (f *fakeStore) GetRateStats(ctx context.Context, currency domain.Currency) (*domain.RateStats, error) {
	return f.stats, f.statsErr
}

func TestGetRates(t *testing.T) {
	store := &fakeStore{
		rates: []domain.Rate{
			{Currency: "BTC", PriceUSD: 65000, FetchedAt: time.Now()},
			{Currency: "ETH", PriceUSD: 3200, FetchedAt: time.Now()},
		},
	}
	h := NewHandler(store, testLogger())

	req := httptest.NewRequest(http.MethodGet, "/rates", nil)
	rec := httptest.NewRecorder()
	h.GetRates(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var got []rateResponse
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d rates, want 2", len(got))
	}
}

func TestGetRates_Empty(t *testing.T) {
	store := &fakeStore{rates: nil}
	h := NewHandler(store, testLogger())

	req := httptest.NewRequest(http.MethodGet, "/rates", nil)
	rec := httptest.NewRecorder()
	h.GetRates(rec, req)

	if got := rec.Body.String(); got != "[]\n" {
		t.Errorf("body = %q, want %q", got, "[]\n")
	}
}

func TestGetRateByCurrency_Found(t *testing.T) {
	change := 1.5
	store := &fakeStore{
		stats: &domain.RateStats{
			Currency:        "BTC",
			PriceUSD:        65000,
			MinUSD24h:       64000,
			MaxUSD24h:       66000,
			FetchedAt:       time.Now(),
			ChangePercent1h: &change,
		},
	}
	h := NewHandler(store, testLogger())
	router := NewRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/rates/BTC", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body: %s", rec.Code, rec.Body.String())
	}

	var got rateStatsResponse
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Currency != "BTC" || got.PriceUSD != 65000 {
		t.Errorf("unexpected response: %+v", got)
	}
	if got.ChangePercent1h == nil || *got.ChangePercent1h != 1.5 {
		t.Errorf("ChangePercent1h = %v, want 1.5", got.ChangePercent1h)
	}
}

func TestGetRateByCurrency_NotFound(t *testing.T) {
	store := &fakeStore{statsErr: storage.ErrNotFound}
	h := NewHandler(store, testLogger())
	router := NewRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/rates/DOGE", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

// Новый тест: невалидный код валюты должен отсекаться на границе HTTP
// (400), даже не доходя до storage это то поведение, которое мы
// добавили в GetRateByCurrency через domain.NewCurrency.
func TestGetRateByCurrency_InvalidCurrency(t *testing.T) {
	store := &fakeStore{}
	h := NewHandler(store, testLogger())
	router := NewRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/rates/123", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}
