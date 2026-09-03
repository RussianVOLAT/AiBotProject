package collector

import (
	"context"
	"log/slog"
	"time"

	"github.com/RussianVOLAT/AiBotProject/internal/domain"
)

// RateInserter минимальный интерфейс, который нужен collectorу от storage.
type RateInserter interface {
	InsertRate(ctx context.Context, currency domain.Currency, price float64) error
}

// PriceFetcher интерфейс над источником цен. Коллектору всё равно,
// откуда берутся цены, реализацию (internal/binance.Client) собираем
// снаружи, в main, и передаём сюда через New.
type PriceFetcher interface {
	FetchPrice(ctx context.Context, symbol string) (float64, error)
}

// symbolMap какие торговые пары источника опрашиваем и в какую domain.Currency
// они превращаются. domain.Currency("BTC") тут собирается напрямую из
// строкового литерала, а не через NewCurrency: значения известны на этапе
// компиляции и заведомо проходят валидацию, вызывать конструктор ради
// констант избыточно.
var symbolMap = map[string]domain.Currency{
	"BTCUSDT": "BTC",
	"ETHUSDT": "ETH",
}

// Collector периодически опрашивает источник цен и сохраняет курсы в storage.
type Collector struct {
	fetcher  PriceFetcher
	storage  RateInserter
	interval time.Duration
	log      *slog.Logger
}

func New(fetcher PriceFetcher, storage RateInserter, interval time.Duration, logger *slog.Logger) *Collector {
	return &Collector{
		fetcher:  fetcher,
		storage:  storage,
		interval: interval,
		log:      logger.With("component", "collector"), // добавляет поле component ко всем логам этого экземпляра
	}
}

func (c *Collector) Run(ctx context.Context) {
	c.collectOnce(ctx)

	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			c.log.Info("stopping, context cancelled")
			return
		case <-ticker.C:
			c.collectOnce(ctx)
		}
	}
}

func (c *Collector) collectOnce(ctx context.Context) {
	for symbol, currency := range symbolMap {
		price, err := c.fetcher.FetchPrice(ctx, symbol)
		if err != nil {
			c.log.Error("fetch failed", "symbol", symbol, "error", err)
			continue
		}

		if err := c.storage.InsertRate(ctx, currency, price); err != nil {
			c.log.Error("insert rate failed", "currency", currency, "error", err)
			continue
		}

		c.log.Info("saved rate", "currency", currency, "price_usd", price)
	}
}
