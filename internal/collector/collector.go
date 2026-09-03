package collector

import (
	"context"
	"log"
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
}

func New(fetcher PriceFetcher, storage RateInserter, interval time.Duration) *Collector {
	return &Collector{
		fetcher:  fetcher,
		storage:  storage,
		interval: interval,
	}
}

func (c *Collector) Run(ctx context.Context) {
	c.collectOnce(ctx)

	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("collector: stopping (context cancelled)")
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
			log.Printf("collector: fetch %s failed: %v", symbol, err)
			continue
		}

		if err := c.storage.InsertRate(ctx, currency, price); err != nil {
			log.Printf("collector: insert rate %s failed: %v", currency, err)
			continue
		}

		log.Printf("collector: saved %s = %.2f USD", currency, price)
	}
}
