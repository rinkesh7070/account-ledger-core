package domain

import "time"

// Account represents a bank account ledger container with an immutable currency precision.
type Account struct {
	ID        string    `json:"id"`
	Currency  Currency  `json:"currency"`
	CreatedAt time.Time `json:"created_at"`
}

// NewAccount creates an account with specified ID and currency.
func NewAccount(id string, currency Currency) Account {
	return Account{
		ID:        id,
		Currency:  currency,
		CreatedAt: time.Now(),
	}
}
