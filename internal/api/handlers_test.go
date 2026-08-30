package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/RussianVOLAT/AiBotProject/internal/storage"
)

// fakeStore  тестовая замена RatesReader, без реальной БД.
// Через неё же удобно проверять "плохие" пути (ошибка, not found),
// которые на живой БД пришлось бы отдельно подстраивать.
type fakeStore struct {
	rates     []storage.Rate
	stats     *storage.RateStats
	statsErr  error
	latestErr error
}

func (f *fakeStore) GetLatestRates(ctx context.Context) ([]storage.Rate, error) {
	return f.rates, f.latestErr
}

func (f *fakeStore) GetRateStats(ctx context.Context, currency string) (*storage.RateStats, error) {
	return f.stats, f.statsErr
}

// numericFromFloat  маленький хелпер, чтобы собрать pgtype.Numeric
// в тестовых данных так же, как это делает pgx при чтении из реальной БД.
func numericFromFloat(t *testing.T, v float64) pgtype.Numeric {
	t.Helper()
	var n pgtype.Numeric
	if err := n.Scan(strconv.FormatFloat(v, 'f', -1, 64)); err != nil {
		t.Fatalf("build numeric: %v", err)
	}
	return n
}
func TestGetRates(t *testing.T) {
	store := &fakeStore{
		rates: []storage.Rate{
			{Currency: "BTC", PriceUSD: numericFromFloat(t, 65000), FetchedAt: time.Now()},
			{Currency: "ETH", PriceUSD: numericFromFloat(t, 3200), FetchedAt: time.Now()},
		},
	}
	h := NewHandler(store)

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
	h := NewHandler(store)

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
		stats: &storage.RateStats{
			Currency:        "BTC",
			PriceUSD:        65000,
			MinUSD24h:       64000,
			MaxUSD24h:       66000,
			FetchedAt:       time.Now(),
			ChangePercent1h: &change,
		},
	}
	h := NewHandler(store)
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
	h := NewHandler(store)
	router := NewRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/rates/DOGE", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}
