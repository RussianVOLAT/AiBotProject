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
// deleteMessage у Telegram работает только для сообщений младше 48 часов.
const maxIntervalMinutes = 24 * 60

// autoIntervalOptions  фиксированный набор интервалов для кнопок быстрого
// выбора. Не ограничивает диапазон  текстовая команда /start-auto {N}
// по-прежнему принимает любое число от 1 до maxIntervalMinutes.
var autoIntervalOptions = []int{5, 15, 30, 60}

// registerHandlers связывает текст команды/callback-данные с обработчиком.
func (b *Bot) registerHandlers() {
	b.tg.RegisterHandler(tgbot.HandlerTypeMessageText, "/start", tgbot.MatchTypeExact, b.handleStart)
	// MatchTypePrefix, а не MatchTypeCommand: последний у этой библиотеки
	// рассчитан на команды с ОБЯЗАТЕЛЬНЫМ аргументом (пример из их доки —
	// "/echo <text>") и не матчит голое "/rates" без ничего после.
	b.tg.RegisterHandler(tgbot.HandlerTypeMessageText, "/rates", tgbot.MatchTypePrefix, b.handleRates)
	b.tg.RegisterHandler(tgbot.HandlerTypeMessageText, "/start-auto", tgbot.MatchTypePrefix, b.handleStartAuto)
	b.tg.RegisterHandler(tgbot.HandlerTypeMessageText, "/stop-auto", tgbot.MatchTypeExact, b.handleStopAuto)

	// Один обработчик на ВСЕ callback-нажатия: пустой префикс матчит любую
	// строку (strings.HasPrefix(s, "") == true всегда), дальше сами
	// разбираем cq.Data по действию.
	b.tg.RegisterHandler(tgbot.HandlerTypeCallbackQueryData, "", tgbot.MatchTypePrefix, b.handleCallbackQuery)
}

// ===== Текстовые команды (оставлены для тех, кто предпочитает их набирать) =====

func (b *Bot) handleStart(ctx context.Context, _ *tgbot.Bot, update *models.Update) {
	const text = "Привет! Я показываю курсы BTC/ETH.\n\n" +
		"Выбери действие кнопкой ниже, или набери команду вручную:\n" +
		"/rates {валюта} — курс + min/max за день и % за час\n" +
		"/start_auto {минуты} — включить автопуш каждые N минут"

	if _, err := b.sendWithKeyboard(ctx, update.Message.Chat.ID, text, mainMenuMarkup()); err != nil {
		b.logger.Error("bot: send /start reply failed", "user_id", update.Message.Chat.ID, "err", err)
	}
}

func (b *Bot) handleRates(ctx context.Context, _ *tgbot.Bot, update *models.Update) {
	chatID := update.Message.Chat.ID
	fields := strings.Fields(update.Message.Text)

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

func (b *Bot) handleStartAuto(ctx context.Context, _ *tgbot.Bot, update *models.Update) {
	chatID := update.Message.Chat.ID
	fields := strings.Fields(update.Message.Text)

	if len(fields) != 2 {
		b.reply(ctx, chatID, "Нужно указать интервал в минутах, например: /start-auto 15")
		return
	}
	minutes, err := strconv.Atoi(fields[1])
	if err != nil {
		b.reply(ctx, chatID, "Интервал должен быть числом минут, например: /start-auto 15")
		return
	}
	b.startAuto(ctx, chatID, minutes)
}

func (b *Bot) handleStopAuto(ctx context.Context, _ *tgbot.Bot, update *models.Update) {
	b.stopAuto(ctx, update.Message.Chat.ID)
}

// handleUnknown — DefaultHandler, срабатывает на текст, не подошедший ни
// под одну команду. Вместо голого "не понял" сразу показываем меню —
// пользователь на расстоянии одного тапа от рабочих кнопок.
func (b *Bot) handleUnknown(ctx context.Context, _ *tgbot.Bot, update *models.Update) {
	if update.Message == nil {
		return
	}
	if _, err := b.sendWithKeyboard(ctx, update.Message.Chat.ID, "Не знаю такую команду. Вот меню:", mainMenuMarkup()); err != nil {
		b.logger.Error("bot: send unknown-command reply failed", "err", err)
	}
}

// ===== Inline-клавиатуры =====

// mainMenuMarkup  стартовое меню после /start (и после любого "не понял").
func mainMenuMarkup() models.ReplyMarkup {
	return &models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{
			{{Text: "📊 Курсы", CallbackData: "menu_rates"}},
			{{Text: "▶️ Включить автопуш", CallbackData: "menu_auto"}},
			{{Text: "⏹ Выключить автопуш", CallbackData: "auto_stop"}},
		},
	}
}

// ratesMenuMarkup список валют для выбора + пункт "все сразу". Список
// не захардкожен: берём его из уже собранных курсов, так что появление
// новой отслеживаемой валюты не требует правки этого файла.
func (b *Bot) ratesMenuMarkup(ctx context.Context) models.ReplyMarkup {
	rates, err := b.rates.GetLatestRates(ctx)
	if err != nil {
		b.logger.Error("bot: build rates menu failed", "err", err)
		rates = nil // покажем хотя бы кнопку "Все" — ошибка уже залогирована
	}

	rows := [][]models.InlineKeyboardButton{
		{{Text: "Все", CallbackData: "rates:ALL"}},
	}
	for _, r := range rates {
		rows = append(rows, []models.InlineKeyboardButton{
			{Text: r.Currency.String(), CallbackData: "rates:" + r.Currency.String()},
		})
	}
	return &models.InlineKeyboardMarkup{InlineKeyboard: rows}
}

// autoMenuMarkup фиксированный набор интервалов автопуша одной строкой.
func autoMenuMarkup() models.ReplyMarkup {
	var row []models.InlineKeyboardButton
	for _, minutes := range autoIntervalOptions {
		row = append(row, models.InlineKeyboardButton{
			Text:         fmt.Sprintf("%d мин", minutes),
			CallbackData: fmt.Sprintf("auto_start:%d", minutes),
		})
	}
	return &models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{row},
	}
}

// handleCallbackQuery единая точка входа для всех нажатий на inline-кнопки.
func (b *Bot) handleCallbackQuery(ctx context.Context, _ *tgbot.Bot, update *models.Update) {
	cq := update.CallbackQuery
	if cq == nil {
		return
	}

	// В приватном чате (один на один с ботом, не группа) ID пользователя
	// совпадает с ID чата  используем его вместо того, чтобы доставать
	// чат из cq.Message: начиная с Bot API 7.x это поле может оказаться
	// "недоступным" сообщением другого типа, возиться с этим не нужно.
	chatID := cq.From.ID

	// Обязательный вызов: снимает "часики" на кнопке в клиенте пользователя
	// независимо от того, что мы дальше решим ответить.
	if _, err := b.tg.AnswerCallbackQuery(ctx, &tgbot.AnswerCallbackQueryParams{
		CallbackQueryID: cq.ID,
	}); err != nil {
		b.logger.Error("bot: answer callback query failed", "err", err)
	}

	// cq.Data формата "действие:аргумент" (например "rates:BTC",
	// "auto_start:15"); для действий без аргумента ("menu_rates") arg
	// после Cut просто останется пустым.
	action, arg, _ := strings.Cut(cq.Data, ":")

	switch action {
	case "menu_rates":
		b.sendWithKeyboard(ctx, chatID, "Выбери валюту:", b.ratesMenuMarkup(ctx))
	case "menu_auto":
		b.sendWithKeyboard(ctx, chatID, "Как часто присылать курс?", autoMenuMarkup())
	case "rates":
		var text string
		if arg == "ALL" {
			text = b.renderAllRates(ctx, chatID)
		} else {
			text = b.renderRateStats(ctx, arg)
		}
		b.reply(ctx, chatID, text)
	case "auto_start":
		minutes, err := strconv.Atoi(arg)
		if err != nil {
			b.reply(ctx, chatID, "Некорректный интервал.")
			return
		}
		b.startAuto(ctx, chatID, minutes)
	case "auto_stop":
		b.stopAuto(ctx, chatID)
	default:
		b.reply(ctx, chatID, "Не знаю такую кнопку.")
	}
}

// ===== Общая логика, вызываемая и из текстовых команд, и из кнопок =====

func (b *Bot) startAuto(ctx context.Context, chatID int64, minutes int) {
	if minutes <= 0 {
		b.reply(ctx, chatID, "Интервал должен быть положительным числом минут.")
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

func (b *Bot) stopAuto(ctx context.Context, chatID int64) {
	if err := b.subs.DeactivateSubscription(ctx, chatID); err != nil {
		b.logger.Error("bot: deactivate subscription failed", "user_id", chatID, "err", err)
		b.reply(ctx, chatID, "Не получилось выключить автопуш, попробуй позже.")
		return
	}
	b.reply(ctx, chatID, "Автопуш выключен.")
}

// ===== Рендеринг текста =====

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
	currency, err := domain.NewCurrency(strings.ToUpper(currencyArg))
	if err != nil {
		return fmt.Sprintf("Не понял валюту %q. Пример: /rates btc", currencyArg)
	}

	stats, err := b.rates.GetRateStats(ctx, currency)
	if err != nil {
		b.logger.Error("bot: fetch rate stats failed", "currency", currency, "err", err)
		return fmt.Sprintf("Не нашёл данных по %s — возможно, эта валюта не отслеживается.", currency)
	}

	return formatRateStats(stats)
}

// formatRateStats  общий формат карточки статистики по одной валюте.
// Используется и в /rates {currency}/кнопке валюты, и в автопуше
// (renderAutopushText в scheduler.go)  чтобы не дублировать форматирование
// в двух местах.
func formatRateStats(stats *domain.RateStats) string {
	// ChangePercent1h указатель: nil значит "мало истории для расчёта",
	// а не "изменение равно нулю" (см. комментарий в domain.RateStats).
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

// reply  тонкая обёртка над send для мест, где нас не интересует messageID
// из ответа.
func (b *Bot) reply(ctx context.Context, chatID int64, text string) {
	if _, err := b.send(ctx, chatID, text); err != nil {
		b.logger.Error("bot: send reply failed", "user_id", chatID, "err", err)
	}
}
