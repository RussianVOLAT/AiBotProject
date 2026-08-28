package collector

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// httptest.NewServer поднимает настоящий локальный HTTP-сервер на случайном
// порту — так тестируем реальный сетевой код (парсинг URL, JSON, статусы),
// но без похода в интернет к настоящему Binance.
func TestBinanceClient_FetchPrice(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("symbol"); got != "BTCUSDT" {
			t.Errorf("unexpected symbol in request: %s", got)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"symbol":"BTCUSDT","price":"65000.12345678"}`))
	}))
	defer server.Close()

	client := NewBinanceClient()
	client.baseURL = server.URL // подменяем реальный Binance на тестовый сервер

	price, err := client.FetchPrice(context.Background(), "BTCUSDT")
	if err != nil {
		t.Fatalf("FetchPrice: %v", err)
	}
	if price != 65000.12345678 {
		t.Errorf("price = %v, want 65000.12345678", price)
	}
}

func TestBinanceClient_FetchPrice_ErrorStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests) // симулируем rate limit от Binance
	}))
	defer server.Close()

	client := NewBinanceClient()
	client.baseURL = server.URL

	_, err := client.FetchPrice(context.Background(), "BTCUSDT")
	if err == nil {
		t.Error("expected error for non-200 status, got nil")
	}
}
