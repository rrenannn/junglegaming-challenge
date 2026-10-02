package domain

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

type Currency string

const BRL Currency = "BRL"

var supportedCurrencies = map[Currency]struct{}{
	BRL: {},
}

func NewCurrency(code string) (Currency, error) {
	normalized := Currency(strings.ToUpper(strings.TrimSpace(code)))
	if !normalized.Valid() {
		return "", NewError(FailureInvalidCurrency, fmt.Sprintf("unsupported currency %q", code))
	}
	return normalized, nil
}

func (c Currency) Valid() bool {
	_, ok := supportedCurrencies[c]
	return ok
}

func (c Currency) String() string {
	return string(c)
}

const moneyScale = int64(100)

var decimalPattern = regexp.MustCompile(`^(0|[1-9]\d*)(\.(\d{1,2}))?$`)

type Money struct {
	amountMinor int64
	currency    Currency
}

func NewMoney(amountMinor int64, currency Currency) (Money, error) {
	if !currency.Valid() {
		return Money{}, NewError(FailureInvalidCurrency, fmt.Sprintf("unsupported currency %q", currency))
	}
	return Money{amountMinor: amountMinor, currency: currency}, nil
}

func ZeroMoney(currency Currency) (Money, error) {
	return NewMoney(0, currency)
}

func ParseMoney(input string, currency Currency) (Money, error) {
	if !currency.Valid() {
		return Money{}, NewError(FailureInvalidCurrency, fmt.Sprintf("unsupported currency %q", currency))
	}

	matches := decimalPattern.FindStringSubmatch(input)
	if matches == nil {
		return Money{}, NewError(FailureInvalidAmount, fmt.Sprintf("invalid amount %q", input))
	}

	integerPart := matches[1]
	fractionPart := matches[3]
	for len(fractionPart) < 2 {
		fractionPart += "0"
	}

	integerValue, err := strconv.ParseInt(integerPart, 10, 64)
	if err != nil {
		return Money{}, NewError(FailureAmountOverflow, fmt.Sprintf("amount %q is out of range", input))
	}
	fractionValue, err := strconv.ParseInt(fractionPart, 10, 64)
	if err != nil {
		return Money{}, NewError(FailureInvalidAmount, fmt.Sprintf("invalid amount %q", input))
	}

	if integerValue > (math.MaxInt64-fractionValue)/moneyScale {
		return Money{}, NewError(FailureAmountOverflow, fmt.Sprintf("amount %q overflows", input))
	}

	return Money{amountMinor: integerValue*moneyScale + fractionValue, currency: currency}, nil
}

func (m Money) Currency() Currency {
	return m.currency
}

func (m Money) AmountMinor() int64 {
	return m.amountMinor
}

func (m Money) IsZero() bool {
	return m.amountMinor == 0
}

func (m Money) IsPositive() bool {
	return m.amountMinor > 0
}

func (m Money) IsNegative() bool {
	return m.amountMinor < 0
}

func (m Money) Add(other Money) (Money, error) {
	if m.currency != other.currency {
		return Money{}, NewError(FailureCurrencyMismatch, "cannot add money with different currencies")
	}
	sum, ok := addInt64(m.amountMinor, other.amountMinor)
	if !ok {
		return Money{}, NewError(FailureAmountOverflow, "amount overflow")
	}
	return Money{amountMinor: sum, currency: m.currency}, nil
}

func (m Money) Sub(other Money) (Money, error) {
	negated, err := other.Negate()
	if err != nil {
		return Money{}, err
	}
	return m.Add(negated)
}

func (m Money) Negate() (Money, error) {
	if m.amountMinor == math.MinInt64 {
		return Money{}, NewError(FailureAmountOverflow, "amount overflow")
	}
	return Money{amountMinor: -m.amountMinor, currency: m.currency}, nil
}

func (m Money) Compare(other Money) (int, error) {
	if m.currency != other.currency {
		return 0, NewError(FailureCurrencyMismatch, "cannot compare money with different currencies")
	}
	switch {
	case m.amountMinor < other.amountMinor:
		return -1, nil
	case m.amountMinor > other.amountMinor:
		return 1, nil
	default:
		return 0, nil
	}
}

func (m Money) Decimal() string {
	sign := ""
	value := m.amountMinor
	if value < 0 {
		sign = "-"
		value = -value
	}
	return fmt.Sprintf("%s%d.%02d", sign, value/moneyScale, value%moneyScale)
}

func (m Money) String() string {
	return fmt.Sprintf("%s %s", m.Decimal(), m.currency)
}

type moneyJSON struct {
	Amount   string   `json:"amount"`
	Currency Currency `json:"currency"`
}

func (m Money) MarshalJSON() ([]byte, error) {
	return json.Marshal(moneyJSON{Amount: m.Decimal(), Currency: m.currency})
}

func (m *Money) UnmarshalJSON(data []byte) error {
	var wire moneyJSON
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	parsed, err := ParseMoney(wire.Amount, wire.Currency)
	if err != nil {
		return err
	}
	*m = parsed
	return nil
}

func addInt64(a, b int64) (int64, bool) {
	sum := a + b
	if (b > 0 && sum < a) || (b < 0 && sum > a) {
		return 0, false
	}
	return sum, true
}
