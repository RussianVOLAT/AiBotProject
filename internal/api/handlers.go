package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/RussianVOLAT/AiBotProject/internal/domain"
	"github.com/RussianVOLAT/AiBotProject/internal/storage"
)

type RatesReader interface {
	GetLatestRates(ctx context.Context) ([]domain.Rate, error)
	GetRateStats(ctx context.Context, currency domain.Currency) (*domain.RateStats, error)
}

type Handler struct {
	store RatesReader
	log   *slog.Logger
}

func NewHandler(store RatesReader, logger *slog.Logger) *Handler {
	return &Handler{
		store: store,
		log:   logger.With("component", "api"),
	}
}

func (h *Handler) GetRates(w http.ResponseWriter, r *http.Request) {
	rates, err := h.store.GetLatestRates(r.Context())
	if err != nil {
		h.log.Error("get latest rates failed", "error", err)
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
		writeError(w, http.StatusBadRequest, "invalid currency code")
		return
	}

	stats, err := h.store.GetRateStats(r.Context(), currency)
	if errors.Is(err, storage.ErrNotFound) {
		writeError(w, http.StatusNotFound, "currency not found")
		return
	}
	if err != nil {
		h.log.Error("get rate stats failed", "currency", currency, "error", err)
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

// writeJSON и writeError логируют через стандартный log единственное
// исключение: эти функции пакетные (не методы Handler), у них нет доступа
// к h.log. Можно было бы сделать их методами, но это раздуло бы сигнатуру
// без реальной необходимости — ошибка кодирования JSON здесь крайне редкий
// случай (по сути, только если сама структура ответа не сериализуется).
func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		slog.Error("api: encode response failed", "error", err)
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, errorResponse{Error: message})
}
