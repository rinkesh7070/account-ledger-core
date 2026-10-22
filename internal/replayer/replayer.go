package replayer

import (
	"fmt"
	"io"

	"github.com/rinkesh7070/account-ledger-core/internal/domain"
	"github.com/rinkesh7070/account-ledger-core/internal/engine"
)

// Replayer orchestrates the 6-day financial simulation against the ledger core.
type Replayer struct {
	ledger       *engine.Ledger
	interestCalc *engine.InterestCalculator
	feeAssessor  *engine.OverdraftFeeAssessor
	reporter     *Reporter
}

// NewReplayer instantiates the configured ledger and replayer pipeline.
func NewReplayer(out io.Writer, policy engine.LedgerPolicy) *Replayer {
	feeAmount, _ := domain.FromDecimalString("25.00", domain.AED)
	feeAssessor := engine.NewOverdraftFeeAssessor(feeAmount)
	interestCalc := engine.NewInterestCalculator(0.0004) // 0.04% per day
	ledger := engine.NewLedger(feeAssessor, interestCalc, policy)

	// Register initial accounts
	ledger.RegisterAccount(domain.NewAccount("ACC-001", domain.AED))
	ledger.RegisterAccount(domain.NewAccount("ACC-002", domain.BHD))

	return &Replayer{
		ledger:       ledger,
		interestCalc: interestCalc,
		feeAssessor:  feeAssessor,
		reporter:     NewReporter(out),
	}
}

// Ledger returns the underlying ledger instance.
func (r *Replayer) Ledger() *engine.Ledger {
	return r.ledger
}

// Run executes the complete 6-day event replay sequence and outputs daily reports.
func (r *Replayer) Run() ([]DailySummary, error) {
	var summaries []DailySummary

	// DAY 1
	// E1: CREDIT ACC-001 AED 1,200.00 — value_date Day 1
	// E2: DEBIT  ACC-001 AED 950.00   — value_date Day 1
	{
		day := 1
		var dayEvents []domain.Event
		var dayErrors []string

		e1Amt, _ := domain.FromDecimalString("1200.00", domain.AED)
		e1, err := r.ledger.PostCredit("E1", "ACC-001", e1Amt, day, 1, "E1 Initial Credit")
		if err != nil {
			dayErrors = append(dayErrors, fmt.Sprintf("E1 failed: %v", err))
		} else {
			dayEvents = append(dayEvents, e1)
		}

		e2Amt, _ := domain.FromDecimalString("950.00", domain.AED)
		e2, err := r.ledger.PostDebit("E2", "ACC-001", e2Amt, day, 1, "E2 Operating Debit")
		if err != nil {
			dayErrors = append(dayErrors, fmt.Sprintf("E2 failed: %v", err))
		} else {
			dayEvents = append(dayEvents, e2)
		}

		summary := r.closeDay(day, dayEvents, dayErrors)
		summaries = append(summaries, summary)
	}

	// DAY 2
	// E3: AUTHORIZATION ACC-001 Auth-A hold AED 200.00 — value_date Day 2
	{
		day := 2
		var dayEvents []domain.Event
		var dayErrors []string

		authAAmt, _ := domain.FromDecimalString("200.00", domain.AED)
		hold, err := r.ledger.RequestAuthorization("Auth-A", "ACC-001", authAAmt, day, 2)
		if err != nil {
			dayErrors = append(dayErrors, fmt.Sprintf("E3 Auth-A declined: %v", err))
		} else {
			dayEvents = append(dayEvents, domain.Event{
				ID:          "E3",
				Type:        domain.EventTypeAuthorization,
				AccountID:   "ACC-001",
				Amount:      authAAmt,
				BookingDay:  day,
				ValueDay:    2,
				AuthID:      hold.AuthID,
				Description: "E3 Authorization Hold Auth-A",
			})
		}

		summary := r.closeDay(day, dayEvents, dayErrors)
		summaries = append(summaries, summary)
	}

	// DAY 3
	// E4: CREDIT ACC-001 AED 400.00 — value_date Day 3
	{
		day := 3
		var dayEvents []domain.Event
		var dayErrors []string

		e4Amt, _ := domain.FromDecimalString("400.00", domain.AED)
		e4, err := r.ledger.PostCredit("E4", "ACC-001", e4Amt, day, 3, "E4 Inbound Credit")
		if err != nil {
			dayErrors = append(dayErrors, fmt.Sprintf("E4 failed: %v", err))
		} else {
			dayEvents = append(dayEvents, e4)
		}

		summary := r.closeDay(day, dayEvents, dayErrors)
		summaries = append(summaries, summary)
	}

	// DAY 4
	// E5: SETTLEMENT ACC-001 Auth-A settles for AED 185.00 — value_date Day 4
	// E6: SETTLEMENT ACC-001 Auth-Z settles for AED 180.00 — value_date Day 4 (No prior authorization)
	{
		day := 4
		var dayEvents []domain.Event
		var dayErrors []string

		// E5: Settle Auth-A
		e5Amt, _ := domain.FromDecimalString("185.00", domain.AED)
		e5, err := r.ledger.SettleAuthorization("E5", "Auth-A", "ACC-001", e5Amt, day, 4)
		if err != nil {
			dayErrors = append(dayErrors, fmt.Sprintf("E5 Auth-A settlement failed: %v", err))
		} else {
			dayEvents = append(dayEvents, e5)
		}

		// E6: Settle Auth-Z (unmatched settlement)
		e6Amt, _ := domain.FromDecimalString("180.00", domain.AED)
		e6, err := r.ledger.SettleAuthorization("E6", "Auth-Z", "ACC-001", e6Amt, day, 4)
		if err != nil {
			dayErrors = append(dayErrors, fmt.Sprintf("E6 Auth-Z settlement rejected: %v (Unmatched authorization)", err))
		} else {
			dayEvents = append(dayEvents, e6)
		}

		summary := r.closeDay(day, dayEvents, dayErrors)
		summaries = append(summaries, summary)
	}

	// DAY 5
	// E7: DEBIT ACC-001 AED 620.00 — value_date Day 2 (BACKDATED!)
	// E8: AUTHORIZATION ACC-001 Auth-B hold AED 90.00 — value_date Day 5
	// E10: CREDIT ACC-002 BHD 10.000, posted as three equal instalments — value_date Day 5
	{
		day := 5
		var dayEvents []domain.Event
		var dayErrors []string

		// E7: Backdated Debit to Day 2
		e7Amt, _ := domain.FromDecimalString("620.00", domain.AED)
		e7, err := r.ledger.PostDebit("E7", "ACC-001", e7Amt, day, 2, "E7 Backdated Merchant Debit to Day 2")
		if err != nil {
			dayErrors = append(dayErrors, fmt.Sprintf("E7 failed: %v", err))
		} else {
			dayEvents = append(dayEvents, e7)
		}

		// E8: Authorization Auth-B hold AED 90.00
		e8Amt, _ := domain.FromDecimalString("90.00", domain.AED)
		holdB, err := r.ledger.RequestAuthorization("Auth-B", "ACC-001", e8Amt, day, 5)
		if err != nil {
			dayErrors = append(dayErrors, fmt.Sprintf("E8 Auth-B DECLINED: %v", err))
		} else {
			dayEvents = append(dayEvents, domain.Event{
				ID:          "E8",
				Type:        domain.EventTypeAuthorization,
				AccountID:   "ACC-001",
				Amount:      e8Amt,
				BookingDay:  day,
				ValueDay:    5,
				AuthID:      holdB.AuthID,
				Description: "E8 Authorization Hold Auth-B",
			})
		}

		// E10: Credit ACC-002 BHD 10.000 in 3 instalments
		e10Total, _ := domain.FromDecimalString("10.000", domain.BHD)
		instalments, err := e10Total.Split(3)
		if err != nil {
			dayErrors = append(dayErrors, fmt.Sprintf("E10 split failed: %v", err))
		} else {
			for i, inst := range instalments {
				instID := fmt.Sprintf("E10-INST-%d", i+1)
				e10Event, err := r.ledger.PostCredit(instID, "ACC-002", inst, day, 5, fmt.Sprintf("E10 Instalment %d/3", i+1))
				if err != nil {
					dayErrors = append(dayErrors, fmt.Sprintf("%s failed: %v", instID, err))
				} else {
					dayEvents = append(dayEvents, e10Event)
				}
			}
		}

		summary := r.closeDay(day, dayEvents, dayErrors)
		summaries = append(summaries, summary)
	}

	// DAY 6
	// E9: REVERSAL ACC-001 reverses E7 — value_date Day 2
	// End of Day 6: Final Interest Capitalization credited to accounts
	{
		day := 6
		var dayEvents []domain.Event
		var dayErrors []string

		// E9: Reversal of E7
		e9, err := r.ledger.ReverseTransaction("E9", "E7", day, 2)
		if err != nil {
			dayErrors = append(dayErrors, fmt.Sprintf("E9 Reversal failed: %v", err))
		} else {
			dayEvents = append(dayEvents, e9)
		}

		summary := r.closeDay(day, dayEvents, dayErrors)
		summaries = append(summaries, summary)
	}

	return summaries, nil
}

// closeDay runs end of day ledger processing and prints the daily report.
func (r *Replayer) closeDay(day int, dayEvents []domain.Event, dayErrors []string) DailySummary {
	generatedEvents, _ := r.ledger.EndOfDayProcessing(day)

	var feesAssessed []domain.Event
	capitalizedInterest := make(map[string]domain.Money)
	for _, ge := range generatedEvents {
		if ge.Type == domain.EventTypeOverdraftFee {
			feesAssessed = append(feesAssessed, ge)
		} else if ge.Type == domain.EventTypeInterestCapitalization {
			capitalizedInterest[ge.AccountID] = ge.Amount
		}
	}

	closingBalances := make(map[string]domain.Money)
	availableBalances := make(map[string]domain.Money)
	interestAccrued := make(map[string]domain.Money)

	accounts := []string{"ACC-001", "ACC-002"}
	for _, accID := range accounts {
		bal, _ := r.ledger.CalculateClosingLedgerBalance(accID, day)
		closingBalances[accID] = bal

		avail, _ := r.ledger.GetAvailableBalance(accID)
		availableBalances[accID] = avail

		// Get daily interest accrued for this day
		accruals := r.interestCalc.GetDailyAccruals(accID)
		for _, acc := range accruals {
			if acc.Day == day {
				interestAccrued[accID] = acc.AccruedAmount
			}
		}
	}

	summary := DailySummary{
		Day:                 day,
		EventsProcessed:     dayEvents,
		ClosingBalances:     closingBalances,
		AvailableBalances:   availableBalances,
		FeesAssessed:        feesAssessed,
		AuthorizationStates: r.ledger.GetAllHolds("ACC-001"),
		Errors:              dayErrors,
		InterestAccrued:     interestAccrued,
		InterestCapitalized: capitalizedInterest,
	}

	r.reporter.PrintDayReport(summary)
	return summary
}
