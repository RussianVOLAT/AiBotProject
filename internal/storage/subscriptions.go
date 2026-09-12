package storage

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/RussianVOLAT/AiBotProject/internal/domain"
)

// UpsertSubscription создаёт подписку или обновляет существующую (по PK user_id).
// Вызывается из хендлера /start-auto — как для первой подписки, так и для
// повторного вызова с другим интервалом.
//
// Осознанно НЕ трогаем last_sent_at/last_message_id при обновлении:
// если пользователь просто меняет interval_minutes, история последней
// отправки не должна сбрасываться — иначе шедулер решит, что подписка
// "новая", и пришлёт курс немедленно, хотя это не всегда ожидаемо.
// active выставляем в true явно — это единственный способ "переактивировать"
// подписку, которую бот раньше отключил из-за 403 (см. DeactivateSubscription).
func (s *Storage) UpsertSubscription(ctx context.Context, userID int64, intervalMinutes int) error {
	const q = `
		INSERT INTO subscriptions (user_id, interval_minutes, active, updated_at)
		VALUES ($1, $2, true, now())
		ON CONFLICT (user_id) DO UPDATE
		SET interval_minutes = EXCLUDED.interval_minutes,
		    active           = true,
		    updated_at       = now()`

	if _, err := s.pool.Exec(ctx, q, userID, intervalMinutes); err != nil {
		return fmt.Errorf("storage: upsert subscription for user %d: %w", userID, err)
	}
	return nil
}

// DeactivateSubscription выключает автопуш (active = false), не удаляя строку —
// история (last_sent_at, last_message_id) остаётся, ей может пригодиться
// диагностика или повторный /start-auto в будущем.
// Используется и для /stop-auto по команде пользователя, и как реакция на 403
// ("бот заблокирован") из шедулера — оба случая ведут себя одинаково с точки
// зрения БД, поэтому один метод на два кейса.
func (s *Storage) DeactivateSubscription(ctx context.Context, userID int64) error {
	const q = `
		UPDATE subscriptions
		SET active = false, updated_at = now()
		WHERE user_id = $1`

	if _, err := s.pool.Exec(ctx, q, userID); err != nil {
		return fmt.Errorf("storage: deactivate subscription for user %d: %w", userID, err)
	}
	return nil
}

// DueSubscriptions возвращает все подписки, которым пора отправить курс —
// active И (ещё ни разу не отправляли ИЛИ прошло достаточно времени).
// SQL-условие — прямая реализация ADR про NULL-багфикс: last_sent_at IS NULL
// проверяем explicitly, потому что "NULL + что угодно" в SQL снова даёт NULL,
// а NULL в WHERE — это всегда "не подходит", а не true/false.
func (s *Storage) DueSubscriptions(ctx context.Context) ([]domain.Subscription, error) {
	const q = `
		SELECT user_id, interval_minutes, active, updated_at, last_sent_at, last_message_id
		FROM subscriptions
		WHERE active = true
		  AND (
		        last_sent_at IS NULL
		        OR last_sent_at + interval_minutes * '1 minute'::interval <= now()
		      )`

	rows, err := s.pool.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("storage: query due subscriptions: %w", err)
	}
	defer rows.Close() // читаем rows вручную (не CollectRows) — тут это не длиннее и явно показывает Scan

	var result []domain.Subscription
	for rows.Next() {
		var sub domain.Subscription
		// LastSentAt и LastMessageID — указатели (*time.Time, *int): pgx v5
		// умеет сканировать NULL прямо в указатель-поле, оставляя его nil,
		// и сам не даёт NULL "просочиться" в non-nullable поля вроде UserID.
		if err := rows.Scan(
			&sub.UserID,
			&sub.IntervalMinutes,
			&sub.Active,
			&sub.UpdatedAt,
			&sub.LastSentAt,
			&sub.LastMessageID,
		); err != nil {
			return nil, fmt.Errorf("storage: scan subscription row: %w", err)
		}
		result = append(result, sub)
	}
	// rows.Err() ловит ошибки, которые могли случиться ПОСЛЕ последнего
	// успешного Next() (например, обрыв соединения на середине выборки) —
	// сам Next() в таких случаях просто вернёт false, без Err() ошибка бы потерялась.
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("storage: iterate due subscriptions: %w", err)
	}
	return result, nil
}

// MarkSent фиксирует факт отправки: новый last_message_id и last_sent_at = sentAt.
// Вызывается сразу после успешного sendMessage, ДО best-effort удаления
// старого сообщения — так подписка консистентна (не "зависнет" со старым
// last_sent_at), даже если следующий шаг (deleteMessage) упадёт.
//
// Намеренно не трогаем updated_at: это поле — про редактирование самой
// подписки пользователем (/start-auto с новым интервалом), а не про работу
// шедулера, см. соответствующий ADR в DECISIONS.md.
func (s *Storage) MarkSent(ctx context.Context, userID int64, messageID int, sentAt time.Time) error {
	const q = `
		UPDATE subscriptions
		SET last_sent_at = $2, last_message_id = $3
		WHERE user_id = $1`

	if _, err := s.pool.Exec(ctx, q, userID, sentAt, messageID); err != nil {
		return fmt.Errorf("storage: mark sent for user %d: %w", userID, err)
	}
	return nil
}

// ErrSubscriptionNotFound — на случай, если понадобится строго отличать
// "подписки нет" от прочих ошибок БД (например, в будущей команде /status).
// Пока в бэклоге такого метода нет, но sentinel-error дёшево завести заранее.
var ErrSubscriptionNotFound = errors.New("storage: subscription not found")

// GetSubscription — точечный лукап одной подписки. Не входит явно в бэклог
// Этапа 2, но такой метод обычно нужен почти сразу (например, для /status
// или для проверки "уже подписан?" перед /start-auto) — дешевле завести
// его сейчас, раз всё равно пишем этот файл.
func (s *Storage) GetSubscription(ctx context.Context, userID int64) (domain.Subscription, error) {
	const q = `
		SELECT user_id, interval_minutes, active, updated_at, last_sent_at, last_message_id
		FROM subscriptions
		WHERE user_id = $1`

	var sub domain.Subscription
	err := s.pool.QueryRow(ctx, q, userID).Scan(
		&sub.UserID,
		&sub.IntervalMinutes,
		&sub.Active,
		&sub.UpdatedAt,
		&sub.LastSentAt,
		&sub.LastMessageID,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Subscription{}, ErrSubscriptionNotFound
	}
	if err != nil {
		return domain.Subscription{}, fmt.Errorf("storage: get subscription for user %d: %w", userID, err)
	}
	return sub, nil
}

// Небольшая заметка про pgxpool.Pool, раз он тут везде: это пул соединений,
// не одно соединение. Exec/Query/QueryRow сами берут свободное соединение
// из пула и возвращают его обратно — вручную Acquire/Release для простых
// запросов не нужен, отсюда и отсутствие явного tx/conn в этом файле.
