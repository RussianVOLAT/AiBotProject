// Package bot реализует Telegram-интерфейс сервиса: команды пользователя
// и фоновый шедулер автопуша курсов.
//
// По аналогии с internal/collector (который не знает про Binance напрямую,
// а работает через свой интерфейс PriceFetcher), bot не завязан на
// конкретный *storage.Storage он объявляет два узких интерфейса под свои
// нужды (RatesStore, SubscriptionStore), которые *storage.Storage
// удовлетворяет "случайно", просто имея нужные методы. Это позволяет
// в тестах подставить фейковую реализацию без поднятия Postgres.
package bot

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
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

	// screenMu/lastScreen трекинг "текущего экрана" на каждый чат для
	// навигации по меню (см. showScreen ниже). Осознанно ТОЛЬКО в памяти,
	// не в БД: это чисто UX-состояние навигации, а не бизнес-данные
	// в отличие от subscriptions.last_message_id (который переживает
	// рестарт контейнера и обязан быть в БД, потому что автопуш работает
	// по расписанию, а не по живому диалогу). Если бот перезапустится,
	// следующее нажатие/команда просто отправит новое сообщение вместо
	// правки старого не баг, а осознанный компромисс простоты.
	screenMu   sync.Mutex
	lastScreen map[int64]int // chatID -> ID последнего "экранного" сообщения бота
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
		lastScreen:    make(map[int64]int),
	}

	opts := []tgbot.Option{
		// DefaultHandler ловит всё, что не подошло ни под одну команду
		// нужен, чтобы бот не молчал на "непонятный текст", а объяснял,
		// что умеет.
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
func (b *Bot) Run(ctx context.Context) {
	go b.runScheduler(ctx)

	// tgbot.Bot.Start блокируется до отмены ctx он сам обрабатывает
	// graceful stop long-polling цикла изнутри библиотеки.
	b.tg.Start(ctx)
}

// send маленькая обёртка вокруг SendMessage без клавиатуры. Используется
// только автопушем (scheduler.go), у которого своё собственное отслеживание
// "последнего сообщения" через subscriptions.last_message_id в БД ему
// showScreen не подходит (это два независимых механизма, см. комментарий
// у lastScreen выше).
func (b *Bot) send(ctx context.Context, chatID int64, text string) (*models.Message, error) {
	return b.sendWithKeyboard(ctx, chatID, text, nil)
}

// sendWithKeyboard как send, но с необязательной inline-клавиатурой.
func (b *Bot) sendWithKeyboard(ctx context.Context, chatID int64, text string, markup models.ReplyMarkup) (*models.Message, error) {
	msg, err := b.tg.SendMessage(ctx, &tgbot.SendMessageParams{
		ChatID:      chatID,
		Text:        text,
		ReplyMarkup: markup,
	})
	if err != nil {
		return nil, err // ошибку не оборачиваем вызывающий код проверяет errors.Is(err, tgbot.ErrorForbidden)
	}
	return msg, nil
}

// showScreen единая точка вывода для интерактивного диалога (команды +
// кнопки). Вместо того чтобы каждый раз слать новое сообщение (то самое
// "плачевно" из переписки десяток сообщений подряд), редактирует то же
// сообщение, что показывали этому чату в прошлый раз, и только если
// редактирование не удалось (первое сообщение в диалоге, старое сообщение
// удалено пользователем и т.п.) отправляет новое и запоминает его ID.
func (b *Bot) showScreen(ctx context.Context, chatID int64, text string, markup models.ReplyMarkup) {
	b.screenMu.Lock()
	prevID, ok := b.lastScreen[chatID]
	b.screenMu.Unlock()

	if ok {
		_, err := b.tg.EditMessageText(ctx, &tgbot.EditMessageTextParams{
			ChatID:      chatID,
			MessageID:   prevID,
			Text:        text,
			ReplyMarkup: markup,
		})
		if err == nil {
			return
		}
		// Не фатально: сообщение могло устареть, быть удалено пользователем,
		// или Telegram вернул "message is not modified" (текст не изменился)
		// в любом из этих случаев просто шлём новое сообщение ниже.
		b.logger.Warn("bot: edit screen failed, sending new message", "user_id", chatID, "err", err)
	}

	msg, err := b.sendWithKeyboard(ctx, chatID, text, markup)
	if err != nil {
		b.logger.Error("bot: send screen failed", "user_id", chatID, "err", err)
		return
	}

	b.screenMu.Lock()
	b.lastScreen[chatID] = msg.ID
	b.screenMu.Unlock()
}
