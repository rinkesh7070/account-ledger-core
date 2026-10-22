package domain

import (
	"fmt"
	"time"
)

// EventType classifies the financial mutation or authorization event.
type EventType string

const (
	EventTypeCredit                 EventType = "CREDIT"
	EventTypeDebit                  EventType = "DEBIT"
	EventTypeAuthorization          EventType = "AUTHORIZATION"
	EventTypeSettlement             EventType = "SETTLEMENT"
	EventTypeReversal               EventType = "REVERSAL"
	EventTypeOverdraftFee           EventType = "OVERDRAFT_FEE"
	EventTypeInterestCapitalization EventType = "INTEREST_CAPITALIZATION"
)

// AuthStatus represents the operational lifecycle of a card authorization hold.
type AuthStatus string

const (
	AuthStatusPending  AuthStatus = "PENDING"
	AuthStatusApproved AuthStatus = "APPROVED"
	AuthStatusDeclined AuthStatus = "DECLINED"
	AuthStatusSettled  AuthStatus = "SETTLED"
	AuthStatusExpired  AuthStatus = "EXPIRED"
)

// Event is the fundamental, immutable record in the ledger.
// It tracks both the booking time (when recorded) and the value date (when economically effective).
type Event struct {
	ID           string            `json:"id"`
	Sequence     int64             `json:"sequence"`
	Type         EventType         `json:"type"`
	AccountID    string            `json:"account_id"`
	Amount       Money             `json:"amount"`
	BookingDay   int               `json:"booking_day"`
	ValueDay     int               `json:"value_day"`
	AuthID       string            `json:"auth_id,omitempty"`
	ReferenceID  string            `json:"reference_id,omitempty"`
	Description  string            `json:"description,omitempty"`
	CreatedAt    time.Time         `json:"created_at"`
	Metadata     map[string]string `json:"metadata,omitempty"`
}

func (e Event) String() string {
	ref := ""
	if e.AuthID != "" {
		ref += fmt.Sprintf(" auth=%s", e.AuthID)
	}
	if e.ReferenceID != "" {
		ref += fmt.Sprintf(" ref=%s", e.ReferenceID)
	}
	return fmt.Sprintf("[%s #%d] Day %d (val Day %d) %s %s %s%s",
		e.ID, e.Sequence, e.BookingDay, e.ValueDay, e.AccountID, e.Type, e.Amount.Format(), ref)
}

// AuthorizationHold tracks an active or historical reservation against an account's available balance.
type AuthorizationHold struct {
	AuthID        string     `json:"auth_id"`
	AccountID     string     `json:"account_id"`
	Amount        Money      `json:"amount"`
	RequestedDay  int        `json:"requested_day"`
	ValueDay      int        `json:"value_day"`
	Status        AuthStatus `json:"status"`
	DeclineReason string     `json:"decline_reason,omitempty"`
	SettledAmount Money      `json:"settled_amount,omitempty"`
	SettledDay    int        `json:"settled_day,omitempty"`
}

// IsActive returns true if the hold is approved and currently reserving available funds.
func (h AuthorizationHold) IsActive() bool {
	return h.Status == AuthStatusApproved
}
