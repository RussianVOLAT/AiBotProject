package domain

import (
	"testing"
	"time"
)

// Table-driven test тот же паттерн, что и в storage/rates_test.go.
// Кейсы намеренно зеркалят SQL-условие из storage.DueSubscriptions
// (last_sent_at IS NULL OR last_sent_at + interval <= now()) плюс
// проверку active, которой в SQL-условии нет явно (там WHERE active = true
// отдельным условием), но которая есть в самом методе IsDue.
func TestSubscriptionIsDue(t *testing.T) {
	now := time.Date(2026, 9, 12, 12, 0, 0, 0, time.UTC)

	recentSentAt := now.Add(-1 * time.Minute)        // 1 минута назад, интервал 5 мин
	exactBoundarySentAt := now.Add(-5 * time.Minute) // ровно интервал назад граница "<="
	longAgoSentAt := now.Add(-10 * time.Minute)      // сильно раньше интервала

	tests := []struct {
		name string
		sub  Subscription
		want bool
	}{
		{
			name: "never sent + active -> due immediately (last_sent_at IS NULL)",
			sub: Subscription{
				Active:          true,
				IntervalMinutes: 5,
				LastSentAt:      nil,
			},
			want: true,
		},
		{
			name: "never sent + inactive -> not due",
			sub: Subscription{
				Active:          false,
				IntervalMinutes: 5,
				LastSentAt:      nil,
			},
			want: false,
		},
		{
			name: "interval not elapsed -> not due",
			sub: Subscription{
				Active:          true,
				IntervalMinutes: 5,
				LastSentAt:      &recentSentAt,
			},
			want: false,
		},
		{
			name: "interval elapsed exactly (boundary <=) -> due",
			sub: Subscription{
				Active:          true,
				IntervalMinutes: 5,
				LastSentAt:      &exactBoundarySentAt,
			},
			want: true,
		},
		{
			name: "interval elapsed long ago -> due",
			sub: Subscription{
				Active:          true,
				IntervalMinutes: 5,
				LastSentAt:      &longAgoSentAt,
			},
			want: true,
		},
		{
			name: "interval elapsed but inactive -> not due",
			sub: Subscription{
				Active:          false,
				IntervalMinutes: 5,
				LastSentAt:      &longAgoSentAt,
			},
			want: false,
		},
		{
			name: "zero interval, never sent -> due",
			sub: Subscription{
				Active:          true,
				IntervalMinutes: 0,
				LastSentAt:      nil,
			},
			want: true,
		},
		{
			name: "zero interval, sent this instant -> due (0 minutes always elapsed)",
			sub: Subscription{
				Active:          true,
				IntervalMinutes: 0,
				LastSentAt:      &now,
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.sub.IsDue(now)
			if got != tt.want {
				t.Errorf("IsDue(%v) = %v, want %v", now, got, tt.want)
			}
		})
	}
}
