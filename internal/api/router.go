package api

import "net/http"

// NewRouter собирает все маршруты api-модуля в http.Handler,
// готовый к передаче в http.ListenAndServe (из main, в следующей сессии).

// Используем стандартный net/http.ServeMux, а не сторонний роутер.
func NewRouter(h *Handler) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /rates", h.GetRates)
	mux.HandleFunc("GET /rates/{currency}", h.GetRateByCurrency)

	return mux
}
