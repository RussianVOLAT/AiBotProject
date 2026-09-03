package api

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"github.com/RussianVOLAT/AiBotProject/internal/domain"
	"github.com/RussianVOLAT/AiBotProject/internal/storage"
)

// RatesReader минимальный интерфейс, который нужен api от storage.
type RatesReader interface {
	GetLatestRates(ctx context.Context) ([]domain.Rate, error)
	GetRateStats(ctx context.Context, currency domain.Currency) (*domain.RateStats, error)
}

type Handler struct {
	store RatesReader
}

func NewHandler(store RatesReader) *Handler {
	return &Handler{store: store}
}

func (h *Handler) GetRates(w http.ResponseWriter, r *http.Request) {
	rates, err := h.store.GetLatestRates(r.Context())
	if err != nil {
		log.Printf("api: get latest rates: %v", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	resp := make([]rateResponse, 0, len(rates))
	for _, rate := range rates {
		resp = append(resp, rateResponse{
			Currency:  rate.Currency.String(),
			PriceUSD:  rate.PriceUSD,
			FetchedAt: rate.FetchedAt,
		})
	}

	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) GetRateByCurrency(w http.ResponseWriter, r *http.Request) {
	currency, err := domain.NewCurrency(r.PathValue("currency"))
	if err != nil {
		// Валидация происходит прямо на границе HTTP если код валюты
		// в URL не проходит по формату domain.Currency, дальше в storage
		// он даже не уйдёт, вернём понятную ошибку клиенту сразу.
		writeError(w, http.StatusBadRequest, "invalid currency code")
		return
	}

	stats, err := h.store.GetRateStats(r.Context(), currency)
	if errors.Is(err, storage.ErrNotFound) {
		writeError(w, http.StatusNotFound, "currency not found")
		return
	}
	if err != nil {
		log.Printf("api: get rate stats for %s: %v", currency, err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	resp := rateStatsResponse{
		Currency:        stats.Currency.String(),
		PriceUSD:        stats.PriceUSD,
		MinUSD24h:       stats.MinUSD24h,
		MaxUSD24h:       stats.MaxUSD24h,
		FetchedAt:       stats.FetchedAt,
		ChangePercent1h: stats.ChangePercent1h,
	}

	writeJSON(w, http.StatusOK, resp)
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("api: encode response: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, errorResponse{Error: message})
}
