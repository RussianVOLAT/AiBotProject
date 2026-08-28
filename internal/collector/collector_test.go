package collector

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// fakeFetcher - тестовая замена PriceFetcher без реальных HTTP-запросов.
// mutex нужен, потому что в реальном Run эти методы могут дёргаться из
// горутины, а тест может читать поля параллельно — на всякий случай
// делаем структуру безопасной для конкурентного доступа с самого начала.
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

// fakeStorage - тестовая замена RateInserter, хранит вставленные записи в памяти
// вместо реальной БД.
type fakeStorage struct {
	mu      sync.Mutex
	inserts map[string]float64
	failOn  map[string]bool
}

func (s *fakeStorage) InsertRate(ctx context.Context, currency string, price float64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failOn[currency] {
		return errors.New("fake insert error")
	}
	if s.inserts == nil {
		s.inserts = make(map[string]float64)
	}
	s.inserts[currency] = price
	return nil
}

func TestCollectOnce_SavesAllCurrencies(t *testing.T) {
	fetcher := &fakeFetcher{prices: map[string]float64{"BTCUSDT": 65000, "ETHUSDT": 3200}}
	storage := &fakeStorage{}

	c := New(fetcher, storage, time.Minute)
	c.collectOnce(context.Background())

	storage.mu.Lock()
	defer storage.mu.Unlock()

	if got := storage.inserts["BTC"]; got != 65000 {
		t.Errorf("BTC price = %v, want 65000", got)
	}
	if got := storage.inserts["ETH"]; got != 3200 {
		t.Errorf("ETH price = %v, want 3200", got)
	}
}

// Проверяем ключевое поведенческое решение: ошибка по одной валюте
// не должна уронить сбор остальных за тот же проход.
func TestCollectOnce_ContinuesAfterFetchError(t *testing.T) {
	fetcher := &fakeFetcher{
		prices: map[string]float64{"BTCUSDT": 65000, "ETHUSDT": 3200},
		failOn: map[string]bool{"BTCUSDT": true},
	}
	storage := &fakeStorage{}

	c := New(fetcher, storage, time.Minute)
	c.collectOnce(context.Background())

	storage.mu.Lock()
	defer storage.mu.Unlock()

	if _, ok := storage.inserts["BTC"]; ok {
		t.Error("BTC should not be saved after fetch error")
	}
	if got := storage.inserts["ETH"]; got != 3200 {
		t.Errorf("ETH price = %v, want 3200", got)
	}
}

func TestCollectOnce_ContinuesAfterInsertError(t *testing.T) {
	fetcher := &fakeFetcher{prices: map[string]float64{"BTCUSDT": 65000, "ETHUSDT": 3200}}
	storage := &fakeStorage{failOn: map[string]bool{"BTC": true}}

	c := New(fetcher, storage, time.Minute)
	c.collectOnce(context.Background())

	storage.mu.Lock()
	defer storage.mu.Unlock()

	if _, ok := storage.inserts["BTC"]; ok {
		t.Error("BTC insert should have failed and not be present")
	}
	if got := storage.inserts["ETH"]; got != 3200 {
		t.Errorf("ETH price = %v, want 3200", got)
	}
}
