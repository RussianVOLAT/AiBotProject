//go:build integration

package storage

import (
	"context"
	"testing"
	"time"
)

// testDSN уже объявлен в integration_test.go (тот же пакет, та же сборка
// по тегу integration) здесь второй раз не объявляем.

// Тестовые user_id заведомо не пересекаются с реальными Telegram chat_id:
// используем отрицательный диапазон, зарезервированный только под тесты.
// Это позволяет гонять тест на общей dev-БД без риска задеть чужие строки
// и без необходимости TRUNCATE всей таблицы (см. cleanup ниже по аналогии
// с тем, как integration_test.go чистит только тестовую валюту TESTCOIN,
// а не всю таблицу rates).
const (
	testUserDueNeverSent    int64 = -900001
	testUserDueOverdue      int64 = -900002
	testUserNotDueRecent    int64 = -900003
	testUserInactiveOverdue int64 = -900004
)

func TestDueSubscriptionsIntegration(t *testing.T) {
	if err := RunMigrations(testDSN); err != nil {
		t.Fatalf("run migrations: %v", err)
	}

	ctx := context.Background()

	st, err := New(ctx, testDSN)
	if err != nil {
		t.Fatalf("connect to storage: %v", err)
	}
	defer st.Close()

	testUserIDs := []int64{
		testUserDueNeverSent,
		testUserDueOverdue,
		testUserNotDueRecent,
		testUserInactiveOverdue,
	}

	cleanup := func() {
		for _, id := range testUserIDs {
			if _, err := st.pool.Exec(ctx, "DELETE FROM subscriptions WHERE user_id = $1", id); err != nil {
				t.Logf("cleanup warning for user %d: %v", id, err)
			}
		}
	}
	cleanup()
	defer cleanup()

	// Кейс 1: только что подписался, ещё ни разу не отправляли.
	// -> due, потому что last_sent_at IS NULL (см. ADR про NULL-багфикс).
	if err := st.UpsertSubscription(ctx, testUserDueNeverSent, 5); err != nil {
		t.Fatalf("upsert never-sent subscription: %v", err)
	}

	// Кейс 2: отправляли час назад, интервал 5 минут давно прошёл.
	// -> due.
	if err := st.UpsertSubscription(ctx, testUserDueOverdue, 5); err != nil {
		t.Fatalf("upsert overdue subscription: %v", err)
	}
	if err := st.MarkSent(ctx, testUserDueOverdue, 111, time.Now().Add(-1*time.Hour)); err != nil {
		t.Fatalf("mark sent (overdue) subscription: %v", err)
	}

	// Кейс 3: отправили прямо сейчас, интервал 60 минут ещё не прошёл.
	// -> не due.
	if err := st.UpsertSubscription(ctx, testUserNotDueRecent, 60); err != nil {
		t.Fatalf("upsert recent subscription: %v", err)
	}
	if err := st.MarkSent(ctx, testUserNotDueRecent, 222, time.Now()); err != nil {
		t.Fatalf("mark sent (recent) subscription: %v", err)
	}

	// Кейс 4: интервал давно прошёл, но подписка неактивна (например, 403
	// от Telegram ранее). -> не due, несмотря на время: WHERE active = true
	// в самом SQL-запросе отсекает такие строки первым условием.
	if err := st.UpsertSubscription(ctx, testUserInactiveOverdue, 5); err != nil {
		t.Fatalf("upsert inactive subscription: %v", err)
	}
	if err := st.MarkSent(ctx, testUserInactiveOverdue, 333, time.Now().Add(-1*time.Hour)); err != nil {
		t.Fatalf("mark sent (inactive) subscription: %v", err)
	}
	if err := st.DeactivateSubscription(ctx, testUserInactiveOverdue); err != nil {
		t.Fatalf("deactivate subscription: %v", err)
	}

	due, err := st.DueSubscriptions(ctx)
	if err != nil {
		t.Fatalf("get due subscriptions: %v", err)
	}

	// Сравниваем не весь список (в общей БД могут быть и другие реальные
	// подписки — как и integration_test.go не проверяет "единственную" валюту
	// в GetLatestRates), а только присутствие/отсутствие наших тестовых id.
	dueIDs := make(map[int64]bool, len(due))
	for _, sub := range due {
		dueIDs[sub.UserID] = true
	}

	wantDue := []int64{testUserDueNeverSent, testUserDueOverdue}
	wantNotDue := []int64{testUserNotDueRecent, testUserInactiveOverdue}

	for _, id := range wantDue {
		if !dueIDs[id] {
			t.Errorf("expected user %d to be in DueSubscriptions result, but it wasn't", id)
		}
	}
	for _, id := range wantNotDue {
		if dueIDs[id] {
			t.Errorf("expected user %d NOT to be in DueSubscriptions result, but it was", id)
		}
	}
}
