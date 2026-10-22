package engine

import (
	"fmt"

	"github.com/rinkesh7070/account-ledger-core/internal/domain"
)

// DailyInterestAccrual records the daily accrued interest for an account on a given calendar day.
type DailyInterestAccrual struct {
	AccountID      string
	Day            int
	ClosingBalance domain.Money
	DailyRate      float64
	AccruedAmount  domain.Money
}

// InterestCalculator manages daily interest accruals and final Day 6 capitalization.
type InterestCalculator struct {
	dailyRate float64
	accruals  map[string]map[int]DailyInterestAccrual // accountID -> day -> accrual
}

// NewInterestCalculator creates an interest calculator with standard daily rate (0.04% = 0.0004).
func NewInterestCalculator(dailyRate float64) *InterestCalculator {
	return &InterestCalculator{
		dailyRate: dailyRate,
		accruals:  make(map[string]map[int]DailyInterestAccrual),
	}
}

// AccrueDay calculates and stores the interest accrued on positive closing ledger balance for day d.
// If balance is zero or negative, zero interest is accrued.
func (ic *InterestCalculator) AccrueDay(accountID string, day int, closingBal domain.Money) DailyInterestAccrual {
	if _, ok := ic.accruals[accountID]; !ok {
		ic.accruals[accountID] = make(map[int]DailyInterestAccrual)
	}

	accrued := domain.RoundInterest(closingBal, ic.dailyRate)
	record := DailyInterestAccrual{
		AccountID:      accountID,
		Day:            day,
		ClosingBalance: closingBal,
		DailyRate:      ic.dailyRate,
		AccruedAmount:  accrued,
	}

	ic.accruals[accountID][day] = record
	return record
}

// GetDailyAccruals returns all recorded daily accruals for an account up to maxDay.
func (ic *InterestCalculator) GetDailyAccruals(accountID string) []DailyInterestAccrual {
	daysMap, ok := ic.accruals[accountID]
	if !ok {
		return nil
	}
	var res []DailyInterestAccrual
	for d := 1; d <= 6; d++ {
		if rec, exists := daysMap[d]; exists {
			res = append(res, rec)
		}
	}
	return res
}

// Capitalize calculates the single capitalization amount at end of Day 6.
// Rule: "The rounded daily accruals must sum exactly to the capitalized total."
func (ic *InterestCalculator) Capitalize(accountID string, currency domain.Currency) (domain.Money, error) {
	total := domain.NewMoney(0, currency)
	daysMap, ok := ic.accruals[accountID]
	if !ok {
		return total, nil
	}

	for d := 1; d <= 6; d++ {
		if rec, exists := daysMap[d]; exists {
			var err error
			total, err = total.Add(rec.AccruedAmount)
			if err != nil {
				return domain.Money{}, fmt.Errorf("capitalization sum error: %w", err)
			}
		}
	}
	return total, nil
}
