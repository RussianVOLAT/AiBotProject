// Package bot реализует Telegram-интерфейс сервиса: команды пользователя
// и фоновый шедулер автопуша курсов.
//
// По аналогии с internal/collector (который не знает про Binance напрямую,
// а работает через свой интерфейс PriceFetcher), bot не завязан на
// конкретный *storage.Storage — он объявляет два узких интерфейса под свои
// нужды (RatesStore, SubscriptionStore), которые *storage.Storage
// удовлетворяет "случайно", просто имея нужные методы. Это позволяет
// в тестах подставить фейковую реализацию без поднятия Postgres.
package bot

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/RussianVOLAT/AiBotProject/internal/domain"
)

// RatesStore то, что боту нужно от хранилища курсов для /rates и /rates {currency}.
type RatesStore interface {
	GetLatestRates(ctx context.Context) ([]domain.Rate, error)
	GetRateStats(ctx context.Context, currency domain.Currency) (*domain.RateStats, error)
}

// SubscriptionStore то, что боту нужно от хранилища подписок.
// Реализуется методами из internal/storage/subscriptions.go.
type SubscriptionStore interface {
	UpsertSubscription(ctx context.Context, userID int64, intervalMinutes int) error
	DeactivateSubscription(ctx context.Context, userID int64) error
	DueSubscriptions(ctx context.Context) ([]domain.Subscription, error)
	MarkSent(ctx context.Context, userID int64, messageID int, sentAt time.Time) error
}

// Bot обёртка над клиентом Telegram Bot API + доступ к storage.
type Bot struct {
	tg     *tgbot.Bot
	rates  RatesStore
	subs   SubscriptionStore
	logger *slog.Logger

	// schedInterval вынесен в поле (а не хардкод в коде шедулера), чтобы
	// в тестах можно было подставить маленький интервал вместо реальной минуты.
	schedInterval time.Duration
}

// New создаёт Bot и регистрирует обработчики команд.
// Сам клиент tgbot.New только настраивает HTTP-клиент и long-polling —
// сетевого запроса на этом шаге ещё нет, он произойдёт в Run().
func New(token string, rates RatesStore, subs SubscriptionStore, logger *slog.Logger) (*Bot, error) {
	b := &Bot{
		rates:         rates,
		subs:          subs,
		logger:        logger,
		schedInterval: time.Minute,
	}

	opts := []tgbot.Option{
		// DefaultHandler ловит всё, что не подошло ни под одну команду —
		// нужен, чтобы бот не молчал на "непонятный текст", а объяснял,
		// что умеет (в частности — на free-text NL-запросы до появления
		// aigateway в Этапе 3).
		tgbot.WithDefaultHandler(b.handleUnknown),
	}

	tg, err := tgbot.New(token, opts...)
	if err != nil {
		return nil, fmt.Errorf("bot: init telegram client: %w", err)
	}
	b.tg = tg
	b.registerHandlers()

	return b, nil
}

// Run запускает бота: фоновый шедулер автопуша в отдельной горутине и
// long polling, который блокирует текущую горутину до отмены ctx.
// Вызывающая сторона (main.go) должна звать Run в своей горутине, как
// уже сделано для collector.Run и http.Server, и передавать общий ctx,
// отменяемый по SIGINT/SIGTERM (см. ADR про cmd/server/main.go).
func (b *Bot) Run(ctx context.Context) {
	go b.runScheduler(ctx)

	// tgbot.Bot.Start блокируется до отмены ctx он сам обрабатывает
	// graceful stop long-polling цикла изнутри библиотеки.
	b.tg.Start(ctx)
}

// send маленькая обёртка вокруг SendMessage, чтобы не тащить
// tgbot.SendMessageParams в каждый хендлер и в шедулер по отдельности.
func (b *Bot) send(ctx context.Context, chatID int64, text string) (*models.Message, error) {
	msg, err := b.tg.SendMessage(ctx, &tgbot.SendMessageParams{
		ChatID: chatID,
		Text:   text,
	})
	if err != nil {
		return nil, err // ошибку не оборачиваем вызывающий код проверяет errors.Is(err, tgbot.ErrorForbidden)
	}
	return msg, nil
}
