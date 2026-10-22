package engine

import (
	"fmt"

	"github.com/rinkesh7070/account-ledger-core/internal/domain"
)

// OverdraftFeeAssessor tracks and generates overdraft fees.
// Non-negotiable rule:
// "Overdraft fee: AED 25.00, assessed once per day per account when that day's
// closing ledger balance (all entries with value_date <= that day) is negative.
// Booked with value_date equal to the day assessed."
type OverdraftFeeAssessor struct {
	feeAmount    domain.Money
	assessedDays map[string]map[int]bool // accountID -> dayAssessed -> bool
}

// NewOverdraftFeeAssessor creates an assessor configured with the mandatory fee amount (AED 25.00).
func NewOverdraftFeeAssessor(feeAmount domain.Money) *OverdraftFeeAssessor {
	return &OverdraftFeeAssessor{
		feeAmount:    feeAmount,
		assessedDays: make(map[string]map[int]bool),
	}
}

// HasFeeBeenAssessed returns true if an overdraft fee has already been assessed for that account on that day.
func (ofa *OverdraftFeeAssessor) HasFeeBeenAssessed(accountID string, day int) bool {
	if days, ok := ofa.assessedDays[accountID]; ok {
		return days[day]
	}
	return false
}

// RecordAssessment marks an overdraft fee as assessed for the given account and day.
func (ofa *OverdraftFeeAssessor) RecordAssessment(accountID string, day int) {
	if _, ok := ofa.assessedDays[accountID]; !ok {
		ofa.assessedDays[accountID] = make(map[int]bool)
	}
	ofa.assessedDays[accountID][day] = true
}

// FeeAmount returns the configured overdraft fee amount.
func (ofa *OverdraftFeeAssessor) FeeAmount() domain.Money {
	return ofa.feeAmount
}

// CreateFeeEvent constructs an immutable domain event for the assessed overdraft fee.
func (ofa *OverdraftFeeAssessor) CreateFeeEvent(accountID string, currentBookingDay int, dayAssessed int, seq int64) domain.Event {
	return domain.Event{
		ID:          fmt.Sprintf("FEE-%s-DAY%d-AT-DAY%d", accountID, dayAssessed, currentBookingDay),
		Sequence:    seq,
		Type:        domain.EventTypeOverdraftFee,
		AccountID:   accountID,
		Amount:      ofa.feeAmount,
		BookingDay:  currentBookingDay,
		ValueDay:    dayAssessed, // Booked with value_date equal to the day assessed
		Description: fmt.Sprintf("Overdraft fee assessed for negative closing balance on Day %d", dayAssessed),
	}
}
