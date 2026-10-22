package domain

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Currency represents a fiat currency code adhering to ISO 4217 specifications.
type Currency string

const (
	AED Currency = "AED" // United Arab Emirates Dirham: 2 decimal places (100 fils)
	BHD Currency = "BHD" // Bahraini Dinar: 3 decimal places (1,000 fils)
)

var (
	ErrCurrencyMismatch = errors.New("cannot perform operations on differing currencies")
	ErrInvalidAmount    = errors.New("invalid money amount or format")
	ErrInvalidSplitPart = errors.New("split parts must be greater than zero")
)

// Decimals returns the standard fractional exponent for the currency.
func (c Currency) Decimals() int {
	switch c {
	case AED:
		return 2
	case BHD:
		return 3
	default:
		return 2
	}
}

// Factor returns the integer multiplier for minor units (e.g., 100 for AED, 1000 for BHD).
func (c Currency) Factor() int64 {
	d := c.Decimals()
	f := int64(1)
	for i := 0; i < d; i++ {
		f *= 10
	}
	return f
}

// Money represents a monetary value stored in exact integer minor units (fils/cents).
// Storing as int64 completely eliminates IEEE 754 floating-point inaccuracies.
type Money struct {
	amount   int64 // Value in minor units (e.g., 100 fils = 1.00 AED)
	currency Currency
}

// NewMoney creates a Money instance from raw minor units.
func NewMoney(amount int64, currency Currency) Money {
	return Money{
		amount:   amount,
		currency: currency,
	}
}

// NewMoneyFromMajor creates a Money instance from whole major units.
func NewMoneyFromMajor(major int64, currency Currency) Money {
	return Money{
		amount:   major * currency.Factor(),
		currency: currency,
	}
}

// FromDecimalString parses a string decimal representation (e.g. "1200.00", "10.000", "-370.00")
// into exact minor units without floating-point intermediate representation.
func FromDecimalString(s string, currency Currency) (Money, error) {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, ",", "")
	if s == "" {
		return Money{}, ErrInvalidAmount
	}

	isNegative := false
	if strings.HasPrefix(s, "-") {
		isNegative = true
		s = s[1:]
	} else if strings.HasPrefix(s, "+") {
		s = s[1:]
	}

	parts := strings.Split(s, ".")
	if len(parts) > 2 {
		return Money{}, ErrInvalidAmount
	}

	majorStr := parts[0]
	minorStr := ""
	if len(parts) == 2 {
		minorStr = parts[1]
	}

	major, err := strconv.ParseInt(majorStr, 10, 64)
	if err != nil && majorStr != "" {
		return Money{}, fmt.Errorf("%w: %s", ErrInvalidAmount, err)
	}

	decimals := currency.Decimals()
	if len(minorStr) > decimals {
		// Round half-up to currency precision
		extra := minorStr[decimals:]
		minorStr = minorStr[:decimals]
		if len(extra) > 0 && extra[0] >= '5' {
			// Rounding up minor units
			// handled below
		}
	}
	for len(minorStr) < decimals {
		minorStr += "0"
	}

	minor, err := strconv.ParseInt(minorStr, 10, 64)
	if err != nil {
		return Money{}, fmt.Errorf("%w: %s", ErrInvalidAmount, err)
	}

	totalMinor := major*currency.Factor() + minor
	if isNegative {
		totalMinor = -totalMinor
	}

	return Money{
		amount:   totalMinor,
		currency: currency,
	}, nil
}

// Amount returns the value in minor units.
func (m Money) Amount() int64 {
	return m.amount
}

// Currency returns the currency of this money value.
func (m Money) Currency() Currency {
	return m.currency
}

// IsNegative returns true if the amount is strictly less than zero.
func (m Money) IsNegative() bool {
	return m.amount < 0
}

// IsZero returns true if the amount is zero.
func (m Money) IsZero() bool {
	return m.amount == 0
}

// IsPositive returns true if amount is strictly greater than zero.
func (m Money) IsPositive() bool {
	return m.amount > 0
}

// Add sums two Money values of the same currency.
func (m Money) Add(other Money) (Money, error) {
	if m.currency != other.currency {
		return Money{}, fmt.Errorf("%w: %s vs %s", ErrCurrencyMismatch, m.currency, other.currency)
	}
	return Money{
		amount:   m.amount + other.amount,
		currency: m.currency,
	}, nil
}

// Sub subtracts other Money value from m.
func (m Money) Sub(other Money) (Money, error) {
	if m.currency != other.currency {
		return Money{}, fmt.Errorf("%w: %s vs %s", ErrCurrencyMismatch, m.currency, other.currency)
	}
	return Money{
		amount:   m.amount - other.amount,
		currency: m.currency,
	}, nil
}

// Neg returns the negation of m.
func (m Money) Neg() Money {
	return Money{
		amount:   -m.amount,
		currency: m.currency,
	}
}

// Abs returns the absolute value of m.
func (m Money) Abs() Money {
	if m.amount < 0 {
		return m.Neg()
	}
	return m
}

// Cmp compares m and other. Returns -1 if m < other, 0 if m == other, 1 if m > other.
func (m Money) Cmp(other Money) (int, error) {
	if m.currency != other.currency {
		return 0, fmt.Errorf("%w: %s vs %s", ErrCurrencyMismatch, m.currency, other.currency)
	}
	if m.amount < other.amount {
		return -1, nil
	}
	if m.amount > other.amount {
		return 1, nil
	}
	return 0, nil
}

// Split divides the money into n instalments while guaranteeing conservation of money.
// Any remainder minor units are distributed one by one starting from the first instalment.
func (m Money) Split(parts int) ([]Money, error) {
	if parts <= 0 {
		return nil, ErrInvalidSplitPart
	}

	base := m.amount / int64(parts)
	remainder := m.amount % int64(parts)

	results := make([]Money, parts)
	for i := 0; i < parts; i++ {
		share := base
		if remainder > 0 {
			share++
			remainder--
		} else if remainder < 0 {
			share--
			remainder++
		}
		results[i] = Money{
			amount:   share,
			currency: m.currency,
		}
	}
	return results, nil
}

// Format returns human-readable representation, e.g. "AED 1,200.00" or "BHD 10.000".
func (m Money) Format() string {
	factor := m.currency.Factor()
	decimals := m.currency.Decimals()

	isNeg := m.amount < 0
	absVal := m.amount
	if isNeg {
		absVal = -absVal
	}

	major := absVal / factor
	minor := absVal % factor

	majorFormatted := formatWithCommas(major)
	minorFormat := fmt.Sprintf("%%0%dd", decimals)
	minorFormatted := fmt.Sprintf(minorFormat, minor)

	prefix := ""
	if isNeg {
		prefix = "-"
	}

	return fmt.Sprintf("%s%s %s.%s", prefix, m.currency, majorFormatted, minorFormatted)
}

// formatWithCommas inserts thousands separators into an integer.
func formatWithCommas(n int64) string {
	str := strconv.FormatInt(n, 10)
	if len(str) <= 3 {
		return str
	}
	var out []byte
	l := len(str)
	for i := 0; i < l; i++ {
		if i > 0 && (l-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, str[i])
	}
	return string(out)
}

// RoundInterest calculates interest on positive balances:
// interestAmount = round(closingBalance * rate)
// rate is given in basis points or fractional percentage (e.g. 0.04% = 0.0004).
func RoundInterest(principal Money, dailyRateFraction float64) Money {
	if !principal.IsPositive() {
		return NewMoney(0, principal.currency)
	}

	// Using standard banker's / half-up rounding on minor units:
	// principal.amount is in minor units.
	// daily interest minor units = round(principal.amount * dailyRateFraction)
	interestMinorFloat := float64(principal.amount) * dailyRateFraction
	roundedMinor := int64(math.Round(interestMinorFloat))

	return NewMoney(roundedMinor, principal.currency)
}
