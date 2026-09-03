package collector

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/RussianVOLAT/AiBotProject/internal/domain"
)

// testLogger логгер для тестов, который никуда не пишет вывод.
// Не хотим засорять вывод go test логами приложения тесты и так
// показывают через t.Log/t.Error всё, что нужно для диагностики.
func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

type fakeFetcher struct {
	mu     sync.Mutex
	prices map[string]float64
	failOn map[string]bool
}

func (f *fakeFetcher) FetchPrice(ctx context.Context, symbol string) (float64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failOn[symbol] {
		return 0, errors.New("fake fetch error")
	}
	return f.prices[symbol], nil
}

type fakeStorage struct {
	mu      sync.Mutex
	inserts map[domain.Currency]float64
	failOn  map[domain.Currency]bool
}

func (s *fakeStorage) InsertRate(ctx context.Context, currency domain.Currency, price float64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failOn[currency] {
		return errors.New("fake insert error")
	}
	if s.inserts == nil {
		s.inserts = make(map[domain.Currency]float64)
	}
	s.inserts[currency] = price
	return nil
}

func TestCollectOnce_SavesAllCurrencies(t *testing.T) {
	fetcher := &fakeFetcher{prices: map[string]float64{"BTCUSDT": 65000, "ETHUSDT": 3200}}
	storage := &fakeStorage{}

	c := New(fetcher, storage, time.Minute, testLogger())
	c.collectOnce(context.Background())

	storage.mu.Lock()
	defer storage.mu.Unlock()

	if got := storage.inserts[domain.Currency("BTC")]; got != 65000 {
		t.Errorf("BTC price = %v, want 65000", got)
	}
	if got := storage.inserts[domain.Currency("ETH")]; got != 3200 {
		t.Errorf("ETH price = %v, want 3200", got)
	}
}

func TestCollectOnce_ContinuesAfterFetchError(t *testing.T) {
	fetcher := &fakeFetcher{
		prices: map[string]float64{"BTCUSDT": 65000, "ETHUSDT": 3200},
		failOn: map[string]bool{"BTCUSDT": true},
	}
	storage := &fakeStorage{}

	c := New(fetcher, storage, time.Minute, testLogger())
	c.collectOnce(context.Background())

	storage.mu.Lock()
	defer storage.mu.Unlock()

	if _, ok := storage.inserts[domain.Currency("BTC")]; ok {
		t.Error("BTC should not be saved after fetch error")
	}
	if got := storage.inserts[domain.Currency("ETH")]; got != 3200 {
		t.Errorf("ETH price = %v, want 3200", got)
	}
}

func TestCollectOnce_ContinuesAfterInsertError(t *testing.T) {
	fetcher := &fakeFetcher{prices: map[string]float64{"BTCUSDT": 65000, "ETHUSDT": 3200}}
	storage := &fakeStorage{failOn: map[domain.Currency]bool{"BTC": true}}

	c := New(fetcher, storage, time.Minute, testLogger())
	c.collectOnce(context.Background())

	storage.mu.Lock()
	defer storage.mu.Unlock()

	if _, ok := storage.inserts[domain.Currency("BTC")]; ok {
		t.Error("BTC insert should have failed and not be present")
	}
	if got := storage.inserts[domain.Currency("ETH")]; got != 3200 {
		t.Errorf("ETH price = %v, want 3200", got)
	}
}
