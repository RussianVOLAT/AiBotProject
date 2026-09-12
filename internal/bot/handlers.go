package bot

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/RussianVOLAT/AiBotProject/internal/domain"
)

// maxIntervalMinutes  верхняя граница interval_minutes для /start-auto.
// Не требование схемы БД, а осознанное ограничение из ADR про автопуш:
// deleteMessage у Telegram работает только для сообщений младше 48 часов,
// при интервале больше этого удаление старого сообщения станет молча
// падать (не фатально, но грязно). 24 часа половина от лимита, с запасом.
const maxIntervalMinutes = 24 * 60

// registerHandlers связывает текст команды с обработчиком.
// MatchTypeExact  для команд без аргументов, MatchTypeCommand  там, где
// после команды идёт текст ("/rates btc", "/start-auto 15"): библиотека сама
// матчит по префиксу "/команда", а разбор оставшегося текста  наша забота
// (см. strings.Fields ниже).
func (b *Bot) registerHandlers() {
	b.tg.RegisterHandler(tgbot.HandlerTypeMessageText, "/start", tgbot.MatchTypeExact, b.handleStart)
	b.tg.RegisterHandler(tgbot.HandlerTypeMessageText, "/rates", tgbot.MatchTypePrefix, b.handleRates)
	b.tg.RegisterHandler(tgbot.HandlerTypeMessageText, "/start_auto", tgbot.MatchTypePrefix, b.handleStartAuto)
	b.tg.RegisterHandler(tgbot.HandlerTypeMessageText, "/stop_auto", tgbot.MatchTypeExact, b.handleStopAuto)
}

func (b *Bot) handleStart(ctx context.Context, _ *tgbot.Bot, update *models.Update) {
	const text = "Привет! Я показываю курсы BTC/ETH.\n\n" +
		"/rates — курсы всех отслеживаемых валют\n" +
		"/rates {валюта} — курс + min/max за день и % за час\n" +
		"/start_auto {минуты} — включить автопуш каждые N минут\n" +
		"/stop_auto — выключить автопуш"

	if _, err := b.send(ctx, update.Message.Chat.ID, text); err != nil {
		b.logger.Error("bot: send /start reply failed", "user_id", update.Message.Chat.ID, "err", err)
	}
}

// handleRates обслуживает и "/rates" (все валюты), и "/rates {currency}"
// (детали по одной) — команда одна, но с разным поведением по числу аргументов.
func (b *Bot) handleRates(ctx context.Context, _ *tgbot.Bot, update *models.Update) {
	chatID := update.Message.Chat.ID
	fields := strings.Fields(update.Message.Text) // ["/rates"] или ["/rates", "btc"]

	var text string
	if len(fields) == 1 {
		text = b.renderAllRates(ctx, chatID)
	} else {
		text = b.renderRateStats(ctx, fields[1])
	}

	if _, err := b.send(ctx, chatID, text); err != nil {
		b.logger.Error("bot: send /rates reply failed", "user_id", chatID, "err", err)
	}
}

func (b *Bot) renderAllRates(ctx context.Context, chatID int64) string {
	rates, err := b.rates.GetLatestRates(ctx)
	if err != nil {
		b.logger.Error("bot: fetch latest rates failed", "user_id", chatID, "err", err)
		return "Не получилось получить курсы, попробуй чуть позже."
	}
	if len(rates) == 0 {
		return "Пока нет ни одного собранного курса."
	}

	var sb strings.Builder
	sb.WriteString("Текущие курсы:\n")
	for _, r := range rates {
		fmt.Fprintf(&sb, "%s: %.2f $\n", r.Currency, r.PriceUSD)
	}
	return sb.String()
}

func (b *Bot) renderRateStats(ctx context.Context, currencyArg string) string {
	// domain.NewCurrency валидирует формат (см. ADR про internal/domain)
	// если пользователь написал что-то не похожее на тикер, узнаём об этом
	// здесь, а не после похода в БД.
	currency, err := domain.NewCurrency(strings.ToUpper(currencyArg))
	if err != nil {
		return fmt.Sprintf("Не понял валюту %q. Пример: /rates btc", currencyArg)
	}

	stats, err := b.rates.GetRateStats(ctx, currency)
	if err != nil {
		b.logger.Error("bot: fetch rate stats failed", "currency", currency, "err", err)
		return fmt.Sprintf("Не нашёл данных по %s — возможно, эта валюта не отслеживается.", currency)
	}
	changeStr := "н/д"
	if stats.ChangePercent1h != nil {
		changeStr = fmt.Sprintf("%.2f", *stats.ChangePercent1h)
	}
	return fmt.Sprintf(
		"%s: %.2f $\nmin/max за день: %.2f / %.2f $\nизменение за час: %s%%",
		stats.Currency, stats.PriceUSD,
		stats.MinUSD24h, stats.MaxUSD24h,
		changeStr,
	)
}

// handleStartAuto включает автопуш. Ожидаемый формат: "/start_auto 15".
func (b *Bot) handleStartAuto(ctx context.Context, _ *tgbot.Bot, update *models.Update) {
	chatID := update.Message.Chat.ID
	fields := strings.Fields(update.Message.Text)

	if len(fields) != 2 {
		b.reply(ctx, chatID, "Нужно указать интервал в минутах, например: /start_auto 15")
		return
	}

	minutes, err := strconv.Atoi(fields[1])
	if err != nil || minutes <= 0 {
		b.reply(ctx, chatID, "Интервал должен быть положительным числом минут, например: /start_auto 15")
		return
	}
	if minutes > maxIntervalMinutes {
		b.reply(ctx, chatID, fmt.Sprintf("Максимальный интервал — %d минут (сутки).", maxIntervalMinutes))
		return
	}

	if err := b.subs.UpsertSubscription(ctx, chatID, minutes); err != nil {
		b.logger.Error("bot: upsert subscription failed", "user_id", chatID, "err", err)
		b.reply(ctx, chatID, "Не получилось включить автопуш, попробуй позже.")
		return
	}

	b.reply(ctx, chatID, fmt.Sprintf("Готово! Буду присылать курс каждые %d мин.", minutes))
}

func (b *Bot) handleStopAuto(ctx context.Context, _ *tgbot.Bot, update *models.Update) {
	chatID := update.Message.Chat.ID

	if err := b.subs.DeactivateSubscription(ctx, chatID); err != nil {
		b.logger.Error("bot: deactivate subscription failed", "user_id", chatID, "err", err)
		b.reply(ctx, chatID, "Не получилось выключить автопуш, попробуй позже.")
		return
	}

	b.reply(ctx, chatID, "Автопуш выключен.")
}

// handleUnknown DefaultHandler: срабатывает на любое сообщение, не
// подошедшее ни под одну зарегистрированную команду. Пропускаем не-текстовые
// апдейты (например, редактирование сообщений) молча они нам не интересны.
func (b *Bot) handleUnknown(ctx context.Context, _ *tgbot.Bot, update *models.Update) {
	if update.Message == nil {
		return
	}
	b.reply(ctx, update.Message.Chat.ID, "Не знаю такой команды. Наберите /start, чтобы увидеть список.")
}

// reply тонкая обёртка над send для мест, где нас не интересует messageID
// из ответа (в отличие от шедулера, где он нужен для последующего MarkSent).
func (b *Bot) reply(ctx context.Context, chatID int64, text string) {
	if _, err := b.send(ctx, chatID, text); err != nil {
		b.logger.Error("bot: send reply failed", "user_id", chatID, "err", err)
	}
}
