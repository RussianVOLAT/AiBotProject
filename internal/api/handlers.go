package api

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"github.com/RussianVOLAT/AiBotProject/internal/storage"
)

// RatesReader  минимальный интерфейс, который нужен api от storage.
// Как и в collector: интерфейс определён в пакете-потребителе, а не
// в storage, и storage.Storage реализует его автоматически, без явного
// "implements" так работает structural typing в Go.
type RatesReader interface {
	GetLatestRates(ctx context.Context) ([]storage.Rate, error)
	GetRateStats(ctx context.Context, currency string) (*storage.RateStats, error)
}

// Handler держит зависимость от storage и реализует HTTP-обработчики.
type Handler struct {
	store RatesReader
}

// NewHandler создаёт Handler поверх любой реализации RatesReader
// (в проде  *storage.Storage, в тестах — фейк).
func NewHandler(store RatesReader) *Handler {
	return &Handler{store: store}
}

// GetRates  GET /rates. Возвращает последний известный курс по каждой валюте.
func (h *Handler) GetRates(w http.ResponseWriter, r *http.Request) {
	rates, err := h.store.GetLatestRates(r.Context())
	if err != nil {
		log.Printf("api: get latest rates: %v", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Осознанно возвращаем [] (пустой JSON-массив), а не null, если rates
	// пуст — так проще клиентам: не нужно отдельно проверять на null
	// перед итерацией. make(..., 0) вместо nil-слайса даёт этот эффект,
	// потому что encoding/json сериализует nil-слайс как null.
	resp := make([]rateResponse, 0, len(rates))
	for _, r := range rates {
		price, err := r.Float64()
		if err != nil {
			log.Printf("api: convert price for %s: %v", r.Currency, err)
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		resp = append(resp, rateResponse{
			Currency:  r.Currency,
			PriceUSD:  price,
			FetchedAt: r.FetchedAt,
		})
	}

	writeJSON(w, http.StatusOK, resp)
}

// GetRateByCurrency GET /rates/{currency}. Возвращает курс + статистику.
func (h *Handler) GetRateByCurrency(w http.ResponseWriter, r *http.Request) {
	// r.PathValue способ достать path-параметр в стандартном net/http
	// начиная с Go 1.22, работает вместе с паттерном "GET /rates/{currency}"
	// в router.go. До 1.22 для этого нужен был сторонний роутер.
	currency := r.PathValue("currency")
	if currency == "" {
		writeError(w, http.StatusBadRequest, "currency is required")
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
		Currency:        stats.Currency,
		PriceUSD:        stats.PriceUSD,
		MinUSD24h:       stats.MinUSD24h,
		MaxUSD24h:       stats.MaxUSD24h,
		FetchedAt:       stats.FetchedAt,
		ChangePercent1h: stats.ChangePercent1h,
	}

	writeJSON(w, http.StatusOK, resp)
}

// writeJSON  общий хелпер сериализации, чтобы Content-Type и код статуса
// выставлялись одинаково во всех обработчиках, а не копипастились.
func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		// На этом этапе заголовки и статус уже отправлены клиенту,
		// исправить ответ нельзя — остаётся только залогировать
		// для диагностики (например, если data не сериализуется в JSON).
		log.Printf("api: encode response: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, errorResponse{Error: message})
}
