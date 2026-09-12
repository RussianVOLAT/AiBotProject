package domain

import "time"

// Subscription — доменная модель подписки на автопуш курсов.
//
// LastSentAt и LastMessageID — указатели, а не time.Time/int напрямую,
// потому что в БД это nullable-колонки: для свежей подписки (сразу после
// /start-auto) их ещё не существует. Указатель = "либо есть значение,
// либо явный nil" — так же, как в SQL NULL != "нулевое значение".
// Альтернатива — sql.NullTime/через отдельный bool-флаг, но в доменном
// слое (в отличие от storage) хочется минимум типов, завязанных на драйвер БД.
type Subscription struct {
	UserID          int64
	IntervalMinutes int
	Active          bool
	UpdatedAt       time.Time
	LastSentAt      *time.Time
	LastMessageID   *int
}

// IsDue сообщает, пора ли отправлять этой подписке новый курс относительно
// момента now. Логика продублирована из SQL-условия в storage — см. ADR
// "Багфикс: NULL-арифметика в условии 'созревшей' подписки" в DECISIONS.md —
// намеренно: реальный отбор "кому шлём" делает WHERE-условие в БД (это и
// эффективнее, и единственный источник истины), а этот метод пригождается
// в юнит-тестах на саму доменную логику без поднятия Postgres.
func (s Subscription) IsDue(now time.Time) bool {
	if !s.Active {
		return false
	}
	if s.LastSentAt == nil {
		return true
	}
	due := s.LastSentAt.Add(time.Duration(s.IntervalMinutes) * time.Minute)
	return !due.After(now) // due <= now
}
