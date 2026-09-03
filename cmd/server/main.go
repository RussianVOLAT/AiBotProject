package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/RussianVOLAT/AiBotProject/internal/api"
	"github.com/RussianVOLAT/AiBotProject/internal/binance"
	"github.com/RussianVOLAT/AiBotProject/internal/collector"
	"github.com/RussianVOLAT/AiBotProject/internal/storage"
)

// config собирает всё, что приложению нужно снаружи (переменные окружения).
// Отдельная структура, а не разбросанные os.Getenv по всему main, так
// весь список внешних зависимостей виден в одном месте.
type config struct {
	databaseURL     string
	httpAddr        string
	collectInterval time.Duration
}

// loadConfig читает конфиг из окружения, подставляя разумные дефолты
// для локальной разработки (совпадают с docker-compose.yml).
func loadConfig() config {
	return config{
		databaseURL:     getEnv("DATABASE_URL", "postgres://appuser:devpassword@localhost:5432/crypto_rates?sslmode=disable"),
		httpAddr:        getEnv("HTTP_ADDR", ":8080"),
		collectInterval: 5 * time.Minute,
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	if err := run(logger); err != nil {
		logger.Error("fatal error", "error", err)
		os.Exit(1)
	}
}

// run вынесен из main отдельной функцией, возвращающей error, так
// main остаётся тонким (только os.Exit), а вся логика тестируема и не
// вызывает os.Exit где попало, что усложнило бы тестирование.
func run(logger *slog.Logger) error {
	cfg := loadConfig()

	// ctx отменяется при получении SIGINT (Ctrl+C) или SIGTERM (сигнал
	// от Docker/systemd при остановке контейнера). Все горутины ниже,
	// слушающие ctx.Done(), из-за этого узнают о необходимости завершиться.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	logger.Info("running migrations")
	if err := storage.RunMigrations(cfg.databaseURL); err != nil {
		return err
	}

	st, err := storage.New(ctx, cfg.databaseURL)
	if err != nil {
		return err
	}
	defer st.Close()

	priceFetcher := binance.New()
	coll := collector.New(priceFetcher, st, cfg.collectInterval, logger)

	handler := api.NewHandler(st, logger)
	router := api.NewRouter(handler)
	server := &http.Server{
		Addr:    cfg.httpAddr,
		Handler: router,
	}

	// errCh собирает первую фатальную ошибку от любой из фоновых горутин
	// (HTTP-сервер упал не из-за штатного Shutdown), чтобы run мог
	// её вернуть и уронить процесс с ненулевым кодом выхода.
	errCh := make(chan error, 1)

	// Горутина 1: collector - фоновый опрос курсов. Run сам блокируется
	// до отмены ctx, поэтому запускаем его в отдельной горутине, чтобы
	// не блокировать остальной main.
	go func() {
		logger.Info("starting collector", "interval", cfg.collectInterval)
		coll.Run(ctx)
	}()

	// Горутина 2: HTTP-сервер.
	go func() {
		logger.Info("starting http server", "addr", cfg.httpAddr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			// http.ErrServerClosed — не настоящая ошибка, а штатный сигнал
			// "сервер остановлен через Shutdown", его специально нужно
			// отфильтровать, иначе graceful shutdown выглядел бы как сбой.
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		logger.Info("shutdown signal received")
	case err := <-errCh:
		logger.Error("http server failed", "error", err)
		return err
	}

	// Даём HTTP-серверу ограниченное время на завершение текущих запросов,
	// а не рубим их мгновенно — это и есть graceful shutdown.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("http server shutdown failed", "error", err)
	}

	logger.Info("shutdown complete")
	return nil
}
