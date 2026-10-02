package domain

import (
	"math"
	"testing"
)

func mustMoney(t *testing.T, amountMinor int64) Money {
	t.Helper()
	m, err := NewMoney(amountMinor, BRL)
	if err != nil {
		t.Fatalf("NewMoney(%d): %v", amountMinor, err)
	}
	return m
}

func TestParseMoney_Valid(t *testing.T) {
	cases := map[string]int64{
		"0":      0,
		"0.00":   0,
		"25":     2500,
		"25.0":   2500,
		"25.00":  2500,
		"25.5":   2550,
		"0.01":   1,
		"999999": 99999900,
	}
	for input, want := range cases {
		got, err := ParseMoney(input, BRL)
		if err != nil {
			t.Fatalf("ParseMoney(%q): unexpected error: %v", input, err)
		}
		if got.AmountMinor() != want {
			t.Fatalf("ParseMoney(%q) = %d, want %d", input, got.AmountMinor(), want)
		}
	}
}

func TestParseMoney_Invalid(t *testing.T) {
	cases := []string{
		"",
		"-25.00",
		"25.001",
		"1e10",
		"NaN",
		"Infinity",
		"25,00",
		" 25.00",
		"25.00 ",
		"abc",
		"00.50",
	}
	for _, input := range cases {
		if _, err := ParseMoney(input, BRL); err == nil {
			t.Fatalf("ParseMoney(%q): expected error, got none", input)
		}
	}
}

func TestParseMoney_InvalidCurrency(t *testing.T) {
	if _, err := ParseMoney("10.00", Currency("XXX")); err == nil {
		t.Fatal("expected error for unsupported currency")
	}
}

func TestNewCurrency_SupportsBRLUSDEUR(t *testing.T) {
	for _, code := range []string{"brl", "USD", " eur "} {
		if _, err := NewCurrency(code); err != nil {
			t.Fatalf("NewCurrency(%q): %v", code, err)
		}
	}
}

func TestParseMoney_Overflow(t *testing.T) {
	if _, err := ParseMoney("99999999999999999999", BRL); err == nil {
		t.Fatal("expected overflow error")
	}
}

func TestMoney_AddSub(t *testing.T) {
	a := mustMoney(t, 1000)
	b := mustMoney(t, 300)

	sum, err := a.Add(b)
	if err != nil || sum.AmountMinor() != 1300 {
		t.Fatalf("Add: got (%v, %v), want 1300", sum, err)
	}

	diff, err := a.Sub(b)
	if err != nil || diff.AmountMinor() != 700 {
		t.Fatalf("Sub: got (%v, %v), want 700", diff, err)
	}
}

func TestMoney_AddOverflow(t *testing.T) {
	a := mustMoney(t, math.MaxInt64)
	b := mustMoney(t, 1)
	if _, err := a.Add(b); err == nil {
		t.Fatal("expected overflow error")
	}
}

func TestMoney_NegateOverflow(t *testing.T) {
	m, err := NewMoney(math.MinInt64, BRL)
	if err != nil {
		t.Fatalf("NewMoney: %v", err)
	}
	if _, err := m.Negate(); err == nil {
		t.Fatal("expected overflow error negating MinInt64")
	}
}

func TestMoney_CurrencyMismatch(t *testing.T) {
	brl := mustMoney(t, 100)
	if _, err := NewMoney(100, Currency("XXX")); err == nil {
		t.Fatal("expected invalid currency error")
	}

	if _, err := brl.Add(Money{amountMinor: 100, currency: "USD"}); err == nil {
		t.Fatal("expected currency mismatch on Add")
	}
	if _, err := brl.Compare(Money{amountMinor: 100, currency: "USD"}); err == nil {
		t.Fatal("expected currency mismatch on Compare")
	}
}

func TestMoney_Compare(t *testing.T) {
	a := mustMoney(t, 100)
	b := mustMoney(t, 200)

	if cmp, _ := a.Compare(b); cmp != -1 {
		t.Fatalf("Compare(100,200) = %d, want -1", cmp)
	}
	if cmp, _ := b.Compare(a); cmp != 1 {
		t.Fatalf("Compare(200,100) = %d, want 1", cmp)
	}
	if cmp, _ := a.Compare(a); cmp != 0 {
		t.Fatalf("Compare(100,100) = %d, want 0", cmp)
	}
}

func TestMoney_DecimalFormatting(t *testing.T) {
	cases := map[int64]string{
		0:    "0.00",
		5:    "0.05",
		2500: "25.00",
		-500: "-5.00",
	}
	for amount, want := range cases {
		m := Money{amountMinor: amount, currency: BRL}
		if got := m.Decimal(); got != want {
			t.Fatalf("Decimal(%d) = %q, want %q", amount, got, want)
		}
	}
}

func TestMoney_JSONRoundTrip(t *testing.T) {
	original := mustMoney(t, 2500)

	data, err := original.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}

	var decoded Money
	if err := decoded.UnmarshalJSON(data); err != nil {
		t.Fatalf("UnmarshalJSON: %v", err)
	}
	if decoded.AmountMinor() != original.AmountMinor() || decoded.Currency() != original.Currency() {
		t.Fatalf("round-trip mismatch: got %+v, want %+v", decoded, original)
	}
}

func TestMoney_JSONRejectsNegative(t *testing.T) {
	var m Money
	err := m.UnmarshalJSON([]byte(`{"amount":"-5.00","currency":"BRL"}`))
	if err == nil {
		t.Fatal("expected error unmarshalling negative amount")
	}
}
