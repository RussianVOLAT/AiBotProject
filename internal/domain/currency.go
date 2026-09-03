// Package domain содержит сущности, независимые от способа хранения
// (Postgres) или способа доставки (HTTP/Telegram). Ни storage, ни api,
// ни collector не должны утекать своими деталями сюда зависимость
// идёт только в одну сторону: от них к domain, никогда наоборот.
package domain

import (
	"errors"
	"fmt"
	"regexp"
)

// ErrInvalidCurrency sentinel ошибка валидации кода валюты.
var ErrInvalidCurrency = errors.New("domain: invalid currency code")

// currencyPattern 2-10 заглавных латинских букв. Осознанно мягкая
// валидация вместо жёсткого enum (BTC/ETH): добавление новой отслеживаемой
// валюты не требует правки этого файла только конфигурации symbolMap
// в internal/collector.
var currencyPattern = regexp.MustCompile(`^[A-Z]{2,10}$`)

// Currency код валюты, например "BTC". Go не даёт строго запретить обход
// NewCurrency для defined-типов на основе string (domain.Currency("xxx")
// компилируется без вызова конструктора) так что это конвенция валидации
// на границах системы (парсинг входа, чтение из БД, разбор HTTP-параметра),
// а не гарантия компилятора.
type Currency string

// NewCurrency валидирует строку и создаёт Currency.
func NewCurrency(code string) (Currency, error) {
	if !currencyPattern.MatchString(code) {
		return "", fmt.Errorf("%w: %q", ErrInvalidCurrency, code)
	}
	return Currency(code), nil
}

// String реализует fmt.Stringer чтобы Currency красиво печаталась в логах
// и легко приводилась к строке там, где это нужно (например, в SQL-запросах).
func (c Currency) String() string {
	return string(c)
}
