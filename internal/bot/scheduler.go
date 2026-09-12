package bot

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	tgbot "github.com/go-telegram/bot"

	"github.com/RussianVOLAT/AiBotProject/internal/domain"
)

// runScheduler — фоновый цикл автопуша. Общая структура — как у
// collector'а (time.Ticker в горутине, остановка по ctx.Done()), только
// интервал в bot фиксированный (раз в минуту проверяем БД), а не "интервал
// = сам процесс сбора", как в collector: конкретные интервалы рассылки —
// это данные подписок, а не тикера.
func (b *Bot) runScheduler(ctx context.Context) {
	ticker := time.NewTicker(b.schedInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			b.logger.Info("bot: scheduler stopped")
			return
		case <-ticker.C:
			b.tick(ctx)
		}
	}
}

// tick — одна итерация шедулера: находим "созревшие" подписки и рассылаем
// каждой курс. Ошибка по одному пользователю не должна останавливать
// рассылку остальным — поэтому все ошибки логируются внутри цикла, а не
// возвращаются наружу.
func (b *Bot) tick(ctx context.Context) {
	due, err := b.subs.DueSubscriptions(ctx)
	if err != nil {
		b.logger.Error("bot: scheduler: fetch due subscriptions failed", "err", err)
		return
	}

	for _, sub := range due {
		b.deliverTo(ctx, sub)
	}
}

// deliverTo реализует порядок из ADR "Автопуш: новое сообщение + удаление
// предыдущего" — порядок шагов важен и намеренно НЕ переставляется:
//
//  1. sendMessage — новое уведомление. Если тут ошибка — подписка просто
//     остаётся в прежнем состоянии, следующий тик попробует снова
//     (кроме случая 403, см. ниже).
//  2. Сразу после успешной отправки — MarkSent (last_sent_at/last_message_id).
//     Порядок важен: если сначала слать, а обновление БД не сделать,
//     next tick снова сочтёт подписку "созревшей" и пришлёт дубликат.
//     Если бы обновляли ДО отправки, а отправка бы упала — подписка
//     считалась бы обслуженной, хотя пользователь ничего не получил.
//  3. Best-effort deleteMessage старого сообщения — по ADR ошибка здесь
//     НЕ фатальна (например, оно старше 48 часов или юзер сам его стёр):
//     логируем и идём дальше, не откатывая шаги 1-2.
func (b *Bot) deliverTo(ctx context.Context, sub domain.Subscription) {
	text := b.renderAutopushText(ctx, time.Now())

	msg, err := b.send(ctx, sub.UserID, text)
	if err != nil {
		if errors.Is(err, tgbot.ErrorForbidden) {
			// Пользователь заблокировал бота — дальнейшие попытки только
			// шумят в логах и жгут rate limit впустую, поэтому отключаем
			// подписку сразу, без ожидания ручного /stop-auto.
			b.logger.Info("bot: user blocked the bot, deactivating subscription", "user_id", sub.UserID)
			if deactivateErr := b.subs.DeactivateSubscription(ctx, sub.UserID); deactivateErr != nil {
				b.logger.Error("bot: deactivate subscription after 403 failed",
					"user_id", sub.UserID, "err", deactivateErr)
			}
			return
		}
		// Любая другая ошибка sendMessage — не трогаем подписку, следующий
		// тик попробует снова с тем же last_sent_at (условие IsDue всё ещё истинно).
		b.logger.Error("bot: scheduler: send message failed", "user_id", sub.UserID, "err", err)
		return
	}

	sentAt := time.Now()
	// msg.ID — идентификатор нового сообщения (models.Message.ID в этой
	// библиотеке хранит telegram message_id). Если у тебя версия пакета
	// именует поле иначе — здесь единственное место, которое нужно поправить.
	if err := b.subs.MarkSent(ctx, sub.UserID, msg.ID, sentAt); err != nil {
		// Отправили, но не смогли обновить БД — состояние временно
		// рассинхронизировано (следующий тик пришлёт ещё одно сообщение
		// раньше срока). Не идеально, но не теряем данные и не роняем цикл;
		// расследуется по логам, если повторяется часто.
		b.logger.Error("bot: scheduler: mark sent failed", "user_id", sub.UserID, "err", err)
		return
	}

	// Удаление старого сообщения — только если оно вообще было
	// (LastMessageID == nil на первой доставке после /start-auto).
	if sub.LastMessageID != nil {
		_, err := b.tg.DeleteMessage(ctx, &tgbot.DeleteMessageParams{
			ChatID:    sub.UserID,
			MessageID: *sub.LastMessageID,
		})
		if err != nil {
			// Ожидаемо и не фатально: сообщение старше 48ч, уже удалено
			// пользователем и т.п. — см. риск в соответствующем ADR.
			b.logger.Warn("bot: delete previous message failed (non-fatal)",
				"user_id", sub.UserID, "message_id", *sub.LastMessageID, "err", err)
		}
	}
}

// renderAutopushText формирует текст автопуша. Метка времени в конце по
// пункту 7 бэклога: не влияет на корректность (это не editMessageText,
// сообщение и так новое), но даёт пользователю видимый сигнал "бот жив",
// даже если курс с прошлого раза не изменился.
func (b *Bot) renderAutopushText(ctx context.Context, now time.Time) string {
	rates, err := b.rates.GetLatestRates(ctx)
	if err != nil || len(rates) == 0 {
		// Не блокируем рассылку из-за временной недоступности данных
		// лучше отправить сообщение "без цифр", чем не отправить вообще
		// (пользователь хотя бы поймёт, что что-то не так, а не решит,
		// что бот перестал работать).
		return fmt.Sprintf("Не получилось получить курсы (%s).", now.Format("15:04:05"))
	}

	var sb strings.Builder
	sb.WriteString("Курсы:\n")
	for _, r := range rates {
		fmt.Fprintf(&sb, "%s: %.2f $\n", r.Currency, r.PriceUSD)
	}
	fmt.Fprintf(&sb, "\nобновлено: %s", now.Format("15:04:05"))
	return sb.String()
}
