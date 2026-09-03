package binance

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

const defaultBaseURL = "https://api.binance.com"

// Client клиент к публичным (не требующим ключа) эндпойнтам Binance.
type Client struct {
	httpClient *http.Client
	baseURL    string
}

// New создаёт клиент с разумным таймаутом на HTTP-запросы.
func New() *Client {
	return &Client{
		httpClient: &http.Client{Timeout: 10 * time.Second},
		baseURL:    defaultBaseURL,
	}
}

// tickerPriceResponse — форма ответа Binance на /api/v3/ticker/price.
// Price — строка, не число! Поэтому парсим
// вручную через strconv.ParseFloat, а не даём encoding/json самому
// распарсить это как float64.
type tickerPriceResponse struct {
	Symbol string `json:"symbol"`
	Price  string `json:"price"`
}

// FetchPrice возвращает текущую цену для торговой пары, например "BTCUSDT".
func (c *Client) FetchPrice(ctx context.Context, symbol string) (float64, error) {
	url := fmt.Sprintf("%s/api/v3/ticker/price?symbol=%s", c.baseURL, symbol)

	// NewRequestWithContext, а не http.Get — так ctx.Done() (например,
	// отмена при shutdown приложения) реально прервёт запрос, а не будет
	// проигнорирован.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, fmt.Errorf("collector: build request for %s: %w", symbol, err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("collector: fetch %s: %w", symbol, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("collector: binance returned status %d for %s", resp.StatusCode, symbol)
	}

	var tr tickerPriceResponse
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return 0, fmt.Errorf("collector: decode response for %s: %w", symbol, err)
	}

	price, err := strconv.ParseFloat(tr.Price, 64)
	if err != nil {
		return 0, fmt.Errorf("collector: parse price %q for %s: %w", tr.Price, symbol, err)
	}

	return price, nil
}
