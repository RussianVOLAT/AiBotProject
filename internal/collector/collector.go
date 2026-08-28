package collector

import (
	"context"
	"log"
	"time"
)

type RateInserter interface {
	InsertRate(ctx context.Context, currency string, price float64) error
}

// PriceFetcher — интерфейс над источником цен. В проде это будет
// *BinanceClient, в тестах — фейк без реальных HTTP-запросов.
type PriceFetcher interface {
	FetchPrice(ctx context.Context, symbol string) (float64, error)
}

var symbolMap = map[string]string{
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

// Run запускает бесконечный цикл опроса. Блокирующий вызов —
// предполагается запуск в отдельной горутине из main (go collector.Run(ctx)).
// Останавливается по отмене ctx — это и есть механизм graceful shutdown:
// main при получении SIGTERM отменяет общий ctx, и все горутины,
// слушающие ctx.Done(), сами аккуратно завершаются.
func (c *Collector) Run(ctx context.Context) {
	// Первый опрос — сразу при старте, не дожидаясь первого тика.
	// Без этого первые 5 минут работы сервиса в БД не будет вообще ничего,
	// а time.NewTicker свой первый сигнал шлёт только через interval.
	c.collectOnce(ctx)

	ticker := time.NewTicker(c.interval)
	defer ticker.Stop() // иначе тикер продолжит жить и после Run — утечка горутины/таймера

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

// collectOnce — один проход по всем отслеживаемым валютам.
// Ошибка по одной валюте не должна останавливать сбор остальных —
// поэтому ошибки логируем и продолжаем (continue), а не делаем return
// из всей функции при первой же неудаче.
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
