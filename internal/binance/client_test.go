package binance

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClient_FetchPrice(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("symbol"); got != "BTCUSDT" {
			t.Errorf("unexpected symbol in request: %s", got)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"symbol":"BTCUSDT","price":"65000.12345678"}`))
	}))
	defer server.Close()

	client := New()
	client.baseURL = server.URL

	price, err := client.FetchPrice(context.Background(), "BTCUSDT")
	if err != nil {
		t.Fatalf("FetchPrice: %v", err)
	}
	if price != 65000.12345678 {
		t.Errorf("price = %v, want 65000.12345678", price)
	}
}

func TestClient_FetchPrice_ErrorStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	client := New()
	client.baseURL = server.URL

	_, err := client.FetchPrice(context.Background(), "BTCUSDT")
	if err == nil {
		t.Error("expected error for non-200 status, got nil")
	}
}
