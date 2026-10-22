package engine

import (
	"fmt"
	"sync"
	"time"

	"github.com/rinkesh7070/account-ledger-core/internal/domain"
)

// LedgerPolicy configures engine rules and tolerances.
type LedgerPolicy struct {
	AllowForcePost bool // If true, settlements without prior authorization (e.g. Visa/Mastercard offline presentments) are debited directly.
}

// Ledger is the thread-safe, in-memory, append-only account ledger core.
type Ledger struct {
	mu           sync.RWMutex
	policy       LedgerPolicy
	accounts     map[string]domain.Account
	events       []domain.Event
	eventSeq     int64
	holds        map[string]domain.AuthorizationHold // authID -> hold
	feeAssessor  *OverdraftFeeAssessor
	interestCalc *InterestCalculator
	closedDays   map[int]bool
}

// NewLedger constructs a new in-memory ledger engine.
func NewLedger(feeAssessor *OverdraftFeeAssessor, interestCalc *InterestCalculator, policy LedgerPolicy) *Ledger {
	return &Ledger{
		policy:       policy,
		accounts:     make(map[string]domain.Account),
		events:       make([]domain.Event, 0),
		holds:        make(map[string]domain.AuthorizationHold),
		feeAssessor:  feeAssessor,
		interestCalc: interestCalc,
		closedDays:   make(map[int]bool),
	}
}

// RegisterAccount adds an account to the ledger.
func (l *Ledger) RegisterAccount(acc domain.Account) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if _, exists := l.accounts[acc.ID]; exists {
		return fmt.Errorf("account %s already registered", acc.ID)
	}
	l.accounts[acc.ID] = acc
	return nil
}

// GetAccount retrieves an account by ID.
func (l *Ledger) GetAccount(accountID string) (domain.Account, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	acc, exists := l.accounts[accountID]
	if !exists {
		return domain.Account{}, domain.ErrAccountNotFound
	}
	return acc, nil
}

// PostCredit records an immutable credit event.
func (l *Ledger) PostCredit(id string, accountID string, amount domain.Money, bookingDay int, valueDay int, desc string) (domain.Event, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	acc, exists := l.accounts[accountID]
	if !exists {
		return domain.Event{}, domain.ErrAccountNotFound
	}
	if amount.Currency() != acc.Currency {
		return domain.Event{}, domain.ErrCurrencyMismatch
	}
	if !amount.IsPositive() {
		return domain.Event{}, domain.ErrNegativeCredit
	}
	if valueDay <= 0 {
		return domain.Event{}, domain.ErrInvalidValueDate
	}

	l.eventSeq++
	event := domain.Event{
		ID:          id,
		Sequence:    l.eventSeq,
		Type:        domain.EventTypeCredit,
		AccountID:   accountID,
		Amount:      amount,
		BookingDay:  bookingDay,
		ValueDay:    valueDay,
		Description: desc,
		CreatedAt:   time.Now(),
	}

	l.events = append(l.events, event)
	return event, nil
}

// PostDebit records an immutable debit event.
func (l *Ledger) PostDebit(id string, accountID string, amount domain.Money, bookingDay int, valueDay int, desc string) (domain.Event, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	acc, exists := l.accounts[accountID]
	if !exists {
		return domain.Event{}, domain.ErrAccountNotFound
	}
	if amount.Currency() != acc.Currency {
		return domain.Event{}, domain.ErrCurrencyMismatch
	}
	if !amount.IsPositive() {
		return domain.Event{}, domain.ErrNegativeDebit
	}
	if valueDay <= 0 {
		return domain.Event{}, domain.ErrInvalidValueDate
	}

	l.eventSeq++
	event := domain.Event{
		ID:          id,
		Sequence:    l.eventSeq,
		Type:        domain.EventTypeDebit,
		AccountID:   accountID,
		Amount:      amount,
		BookingDay:  bookingDay,
		ValueDay:    valueDay,
		Description: desc,
		CreatedAt:   time.Now(),
	}

	l.events = append(l.events, event)
	return event, nil
}

// RequestAuthorization evaluates and holds funds.
// Non-negotiable rule:
// "An authorization is approved only if the account's available balance — ledger
// balance minus active holds — remains at or above zero after the hold is applied."
func (l *Ledger) RequestAuthorization(authID string, accountID string, amount domain.Money, bookingDay int, valueDay int) (domain.AuthorizationHold, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	acc, exists := l.accounts[accountID]
	if !exists {
		return domain.AuthorizationHold{}, domain.ErrAccountNotFound
	}
	if amount.Currency() != acc.Currency {
		return domain.AuthorizationHold{}, domain.ErrCurrencyMismatch
	}

	// Current available balance = current total posted ledger balance - active holds
	avail := l.calculateAvailableBalanceLocked(accountID)
	remaining, err := avail.Sub(amount)
	if err != nil {
		return domain.AuthorizationHold{}, err
	}

	hold := domain.AuthorizationHold{
		AuthID:       authID,
		AccountID:    accountID,
		Amount:       amount,
		RequestedDay: bookingDay,
		ValueDay:     valueDay,
	}

	if remaining.IsNegative() {
		hold.Status = domain.AuthStatusDeclined
		hold.DeclineReason = fmt.Sprintf("available balance %s is insufficient for hold %s", avail.Format(), amount.Format())
		l.holds[authID] = hold
		return hold, fmt.Errorf("%w: available %s, requested hold %s", domain.ErrInsufficientAvailableFunds, avail.Format(), amount.Format())
	}

	hold.Status = domain.AuthStatusApproved
	l.holds[authID] = hold

	// Record authorization in audit event stream (does not impact ledger balance)
	l.eventSeq++
	l.events = append(l.events, domain.Event{
		ID:          fmt.Sprintf("AUTH-%s", authID),
		Sequence:    l.eventSeq,
		Type:        domain.EventTypeAuthorization,
		AccountID:   accountID,
		Amount:      amount,
		BookingDay:  bookingDay,
		ValueDay:    valueDay,
		AuthID:      authID,
		Description: fmt.Sprintf("Authorization hold approved for %s", authID),
		CreatedAt:   time.Now(),
	})

	return hold, nil
}

// SettleAuthorization processes a settlement for a prior authorization or force post.
func (l *Ledger) SettleAuthorization(eventID string, authID string, accountID string, settleAmount domain.Money, bookingDay int, valueDay int) (domain.Event, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	acc, exists := l.accounts[accountID]
	if !exists {
		return domain.Event{}, domain.ErrAccountNotFound
	}
	if settleAmount.Currency() != acc.Currency {
		return domain.Event{}, domain.ErrCurrencyMismatch
	}

	hold, holdExists := l.holds[authID]
	if !holdExists {
		if !l.policy.AllowForcePost {
			return domain.Event{}, fmt.Errorf("%w: authID %s not found in ledger", domain.ErrAuthorizationNotFound, authID)
		}
		// Force Post: settle without prior authorization (card network scheme clearing rule)
	} else {
		if hold.Status != domain.AuthStatusApproved {
			return domain.Event{}, fmt.Errorf("%w: authID %s status is %s", domain.ErrAuthorizationAlreadyClosed, authID, hold.Status)
		}
		// Release the hold and mark settled
		hold.Status = domain.AuthStatusSettled
		hold.SettledAmount = settleAmount
		hold.SettledDay = bookingDay
		l.holds[authID] = hold
	}

	// Book financial debit entry to the ledger
	l.eventSeq++
	event := domain.Event{
		ID:          eventID,
		Sequence:    l.eventSeq,
		Type:        domain.EventTypeSettlement,
		AccountID:   accountID,
		Amount:      settleAmount,
		BookingDay:  bookingDay,
		ValueDay:    valueDay,
		AuthID:      authID,
		Description: fmt.Sprintf("Settlement for %s of amount %s", authID, settleAmount.Format()),
		CreatedAt:   time.Now(),
	}

	l.events = append(l.events, event)
	return event, nil
}

// ReverseTransaction records an immutable reversal event for a prior financial entry.
// Non-negotiable rule: "The ledger is append-only. No event record is ever mutated or deleted."
func (l *Ledger) ReverseTransaction(eventID string, targetEventID string, bookingDay int, valueDay int) (domain.Event, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	var targetEvent *domain.Event
	for i := range l.events {
		if l.events[i].ID == targetEventID {
			targetEvent = &l.events[i]
			break
		}
	}
	if targetEvent == nil {
		return domain.Event{}, fmt.Errorf("%w: %s", domain.ErrTransactionNotFound, targetEventID)
	}

	// Check if already reversed
	for _, e := range l.events {
		if e.Type == domain.EventTypeReversal && e.ReferenceID == targetEventID {
			return domain.Event{}, fmt.Errorf("%w: %s", domain.ErrTransactionAlreadyReversed, targetEventID)
		}
	}

	// Construct reversal entry
	l.eventSeq++
	revEvent := domain.Event{
		ID:          eventID,
		Sequence:    l.eventSeq,
		Type:        domain.EventTypeReversal,
		AccountID:   targetEvent.AccountID,
		Amount:      targetEvent.Amount,
		BookingDay:  bookingDay,
		ValueDay:    valueDay,
		ReferenceID: targetEventID,
		Description: fmt.Sprintf("Reversal of %s (%s)", targetEventID, targetEvent.Amount.Format()),
		CreatedAt:   time.Now(),
	}

	l.events = append(l.events, revEvent)
	return revEvent, nil
}

// CalculateClosingLedgerBalance returns the closing balance on day asOfValueDay:
// Rule: "(all entries with value_date <= that day)"
func (l *Ledger) CalculateClosingLedgerBalance(accountID string, asOfValueDay int) (domain.Money, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	return l.calculateClosingLedgerBalanceLocked(accountID, asOfValueDay)
}

// calculateClosingLedgerBalanceLocked computes closing ledger balance with lock held.
func (l *Ledger) calculateClosingLedgerBalanceLocked(accountID string, asOfValueDay int) (domain.Money, error) {
	acc, exists := l.accounts[accountID]
	if !exists {
		return domain.Money{}, domain.ErrAccountNotFound
	}

	bal := domain.NewMoney(0, acc.Currency)
	for _, e := range l.events {
		if e.AccountID != accountID {
			continue
		}
		if e.ValueDay > asOfValueDay {
			continue
		}

		var err error
		switch e.Type {
		case domain.EventTypeCredit, domain.EventTypeInterestCapitalization:
			bal, err = bal.Add(e.Amount)
		case domain.EventTypeDebit, domain.EventTypeSettlement, domain.EventTypeOverdraftFee:
			bal, err = bal.Sub(e.Amount)
		case domain.EventTypeReversal:
			// If target was debit/fee/settlement, reversal adds back funds.
			// Look up target:
			targetType := l.lookupEventType(e.ReferenceID)
			if targetType == domain.EventTypeDebit || targetType == domain.EventTypeSettlement || targetType == domain.EventTypeOverdraftFee {
				bal, err = bal.Add(e.Amount)
			} else {
				bal, err = bal.Sub(e.Amount)
			}
		case domain.EventTypeAuthorization:
			// Authorization does not affect ledger balance!
		}
		if err != nil {
			return domain.Money{}, err
		}
	}

	return bal, nil
}

// calculateAvailableBalanceLocked returns current total ledger balance minus active holds.
func (l *Ledger) calculateAvailableBalanceLocked(accountID string) domain.Money {
	acc := l.accounts[accountID]
	ledgerBal := domain.NewMoney(0, acc.Currency)

	// Sum all entries posted to date
	for _, e := range l.events {
		if e.AccountID != accountID {
			continue
		}
		switch e.Type {
		case domain.EventTypeCredit, domain.EventTypeInterestCapitalization:
			ledgerBal, _ = ledgerBal.Add(e.Amount)
		case domain.EventTypeDebit, domain.EventTypeSettlement, domain.EventTypeOverdraftFee:
			ledgerBal, _ = ledgerBal.Sub(e.Amount)
		case domain.EventTypeReversal:
			targetType := l.lookupEventType(e.ReferenceID)
			if targetType == domain.EventTypeDebit || targetType == domain.EventTypeSettlement || targetType == domain.EventTypeOverdraftFee {
				ledgerBal, _ = ledgerBal.Add(e.Amount)
			} else {
				ledgerBal, _ = ledgerBal.Sub(e.Amount)
			}
		}
	}

	// Subtract active holds
	totalHolds := domain.NewMoney(0, acc.Currency)
	for _, h := range l.holds {
		if h.AccountID == accountID && h.IsActive() {
			totalHolds, _ = totalHolds.Add(h.Amount)
		}
	}

	avail, _ := ledgerBal.Sub(totalHolds)
	return avail
}

// GetAvailableBalance returns current available balance for an account.
func (l *Ledger) GetAvailableBalance(accountID string) (domain.Money, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	if _, exists := l.accounts[accountID]; !exists {
		return domain.Money{}, domain.ErrAccountNotFound
	}
	return l.calculateAvailableBalanceLocked(accountID), nil
}

// GetActiveHolds returns all currently active holds for an account.
func (l *Ledger) GetActiveHolds(accountID string) []domain.AuthorizationHold {
	l.mu.RLock()
	defer l.mu.RUnlock()

	var active []domain.AuthorizationHold
	for _, h := range l.holds {
		if h.AccountID == accountID && h.IsActive() {
			active = append(active, h)
		}
	}
	return active
}

// GetAllHolds returns all holds (approved, declined, settled) for an account.
func (l *Ledger) GetAllHolds(accountID string) []domain.AuthorizationHold {
	l.mu.RLock()
	defer l.mu.RUnlock()

	var all []domain.AuthorizationHold
	for _, h := range l.holds {
		if h.AccountID == accountID {
			all = append(all, h)
		}
	}
	return all
}

// AssessOverdraftFees evaluates days up to currentDay and generates overdraft fees for any
// day where closing balance is negative and no fee has been assessed yet for that day.
func (l *Ledger) AssessOverdraftFees(currentBookingDay int) ([]domain.Event, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	var feeEvents []domain.Event

	for accountID, acc := range l.accounts {
		// Only accounts in AED incur AED 25 overdraft fee per spec
		if acc.Currency != domain.AED {
			continue
		}

		for day := 1; day <= currentBookingDay; day++ {
			if l.feeAssessor.HasFeeBeenAssessed(accountID, day) {
				continue
			}

			closingBal, err := l.calculateClosingLedgerBalanceLocked(accountID, day)
			if err != nil {
				return nil, err
			}

			if closingBal.IsNegative() {
				// Assess fee
				l.feeAssessor.RecordAssessment(accountID, day)
				l.eventSeq++
				feeEvent := l.feeAssessor.CreateFeeEvent(accountID, currentBookingDay, day, l.eventSeq)
				l.events = append(l.events, feeEvent)
				feeEvents = append(feeEvents, feeEvent)
			}
		}
	}

	return feeEvents, nil
}

// EndOfDayProcessing executes the daily closing pipeline:
// 1. Assesses overdraft fees for negative closing balances.
// 2. Accrues daily interest on positive closing ledger balances.
// 3. At Day 6, capitalizes total interest as a single credit entry.
func (l *Ledger) EndOfDayProcessing(day int) ([]domain.Event, error) {
	// First assess any cascading or new overdraft fees
	feeEvents, err := l.AssessOverdraftFees(day)
	if err != nil {
		return nil, err
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	var capitalizationEvents []domain.Event

	// Daily interest accrual for each account
	for accountID, acc := range l.accounts {
		closingBal, err := l.calculateClosingLedgerBalanceLocked(accountID, day)
		if err != nil {
			return nil, err
		}

		l.interestCalc.AccrueDay(accountID, day, closingBal)

		// On Day 6, capitalize total accrued interest
		if day == 6 {
			totalInterest, err := l.interestCalc.Capitalize(accountID, acc.Currency)
			if err != nil {
				return nil, err
			}
			if totalInterest.IsPositive() {
				l.eventSeq++
				capEvent := domain.Event{
					ID:          fmt.Sprintf("INT-CAP-%s-DAY6", accountID),
					Sequence:    l.eventSeq,
					Type:        domain.EventTypeInterestCapitalization,
					AccountID:   accountID,
					Amount:      totalInterest,
					BookingDay:  6,
					ValueDay:    6,
					Description: fmt.Sprintf("Capitalized 6-day interest total of %s", totalInterest.Format()),
					CreatedAt:   time.Now(),
				}
				l.events = append(l.events, capEvent)
				capitalizationEvents = append(capitalizationEvents, capEvent)
			}
		}
	}

	allGenerated := append(feeEvents, capitalizationEvents...)
	return allGenerated, nil
}

// GetEvents returns a copy of all events in chronological sequence.
func (l *Ledger) GetEvents() []domain.Event {
	l.mu.RLock()
	defer l.mu.RUnlock()

	copied := make([]domain.Event, len(l.events))
	copy(copied, l.events)
	return copied
}

func (l *Ledger) lookupEventType(eventID string) domain.EventType {
	for _, e := range l.events {
		if e.ID == eventID {
			return e.Type
		}
	}
	return ""
}
