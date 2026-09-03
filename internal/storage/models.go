package storage

import (
	"fmt"
	"time"

	"github.com/RussianVOLAT/AiBotProject/internal/domain"
	"github.com/jackc/pgx/v5/pgtype"
)

// dbRate сырая строка таблицы rates, как её видит pgx. Понимает только
// storage; наружу утекает domain.Rate, конвертация происходит в toDomain —
// это и есть граница между "деталями Postgres" и "доменной моделью".
type dbRate struct {
	ID        int64
	Currency  string
	PriceUSD  pgtype.Numeric
	FetchedAt time.Time
}

// toDomain конвертирует сырую строку БД в доменную модель, валидируя
// код валюты через domain.NewCurrency если в БД каким-то образом
// оказалась "мусорная" валюта, узнаем об этом здесь, а не в api.
func (r dbRate) toDomain() (domain.Rate, error) {
	currency, err := domain.NewCurrency(r.Currency)
	if err != nil {
		return domain.Rate{}, fmt.Errorf("storage: invalid currency in db: %w", err)
	}

	price, err := numericToFloat64(r.PriceUSD)
	if err != nil {
		return domain.Rate{}, fmt.Errorf("storage: convert price: %w", err)
	}

	return domain.Rate{
		Currency:  currency,
		PriceUSD:  price,
		FetchedAt: r.FetchedAt,
	}, nil
}

// numericToFloat64 единственное место в storage, где мы явно теряем
// часть точности decimal ради удобства работы дальше по системе.
// Для реальных финансовых операций так делать нельзя, для отображения
// курса пользователю норм.
func numericToFloat64(n pgtype.Numeric) (float64, error) {
	f, err := n.Float64Value()
	if err != nil {
		return 0, err
	}
	return f.Float64, nil
}
