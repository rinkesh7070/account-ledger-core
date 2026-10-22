package replayer

import (
	"fmt"
	"io"
	"strings"

	"github.com/rinkesh7070/account-ledger-core/internal/domain"
)

// DailySummary encapsulates the audit state of the ledger for a specific calendar day.
type DailySummary struct {
	Day                 int
	EventsProcessed     []domain.Event
	ClosingBalances     map[string]domain.Money // accountID -> closing ledger balance
	AvailableBalances   map[string]domain.Money // accountID -> available balance
	FeesAssessed        []domain.Event          // Overdraft fee events generated on/for this day
	AuthorizationStates []domain.AuthorizationHold
	Errors              []string
	InterestAccrued     map[string]domain.Money
	InterestCapitalized map[string]domain.Money
}

// Reporter formats and prints the daily ledger state.
type Reporter struct {
	writer io.Writer
}

// NewReporter creates a new reporter printing to w.
func NewReporter(w io.Writer) *Reporter {
	return &Reporter{writer: w}
}

// PrintDayReport renders a formatted daily ledger summary.
func (r *Reporter) PrintDayReport(s DailySummary) {
	sep := strings.Repeat("=", 80)
	subSep := strings.Repeat("-", 80)

	fmt.Fprintf(r.writer, "\n%s\n", sep)
	fmt.Fprintf(r.writer, "                       DAY %d LEDGER CLOSE REPORT\n", s.Day)
	fmt.Fprintf(r.writer, "%s\n", sep)

	// 1. Events Processed
	fmt.Fprintf(r.writer, "1. BOOKED EVENTS PROCESSED TODAY (%d events):\n", len(s.EventsProcessed))
	if len(s.EventsProcessed) == 0 {
		fmt.Fprintf(r.writer, "   (No events booked on Day %d)\n", s.Day)
	} else {
		for _, e := range s.EventsProcessed {
			fmt.Fprintf(r.writer, "   • %s\n", e.String())
		}
	}
	fmt.Fprintf(r.writer, "%s\n", subSep)

	// 2. Closing Ledger & Available Balances
	fmt.Fprintf(r.writer, "2. ACCOUNT BALANCES AT CLOSE OF DAY %d:\n", s.Day)
	for accID, bal := range s.ClosingBalances {
		avail := s.AvailableBalances[accID]
		interest := s.InterestAccrued[accID]
		fmt.Fprintf(r.writer, "   • Account: %-8s | Closing Ledger Balance: %14s | Available Balance: %14s | Day Interest Accrued: %10s\n",
			accID, bal.Format(), avail.Format(), interest.Format())
	}
	fmt.Fprintf(r.writer, "%s\n", subSep)

	// 3. Fee Assessments
	fmt.Fprintf(r.writer, "3. FEE ASSESSMENTS GENERATED TODAY (%d fees):\n", len(s.FeesAssessed))
	if len(s.FeesAssessed) == 0 {
		fmt.Fprintf(r.writer, "   • None (No accounts in overdraft)\n")
	} else {
		for _, f := range s.FeesAssessed {
			fmt.Fprintf(r.writer, "   • [OVERDRAFT FEE] Account: %s | Amount: %s | Booked: Day %d | Value Date: Day %d | Reason: %s\n",
				f.AccountID, f.Amount.Format(), f.BookingDay, f.ValueDay, f.Description)
		}
	}
	fmt.Fprintf(r.writer, "%s\n", subSep)

	// 4. Authorization States
	fmt.Fprintf(r.writer, "4. AUTHORIZATION STATES (Active & Historical Holds):\n")
	if len(s.AuthorizationStates) == 0 {
		fmt.Fprintf(r.writer, "   • (No authorizations recorded)\n")
	} else {
		for _, h := range s.AuthorizationStates {
			settleInfo := ""
			if h.Status == domain.AuthStatusSettled {
				settleInfo = fmt.Sprintf(" (Settled for %s on Day %d)", h.SettledAmount.Format(), h.SettledDay)
			} else if h.Status == domain.AuthStatusDeclined {
				settleInfo = fmt.Sprintf(" (Decline Reason: %s)", h.DeclineReason)
			}
			fmt.Fprintf(r.writer, "   • Hold [%s] | Account: %s | Amount: %s | Status: %-8s%s\n",
				h.AuthID, h.AccountID, h.Amount.Format(), h.Status, settleInfo)
		}
	}
	fmt.Fprintf(r.writer, "%s\n", subSep)

	// 5. Errors & Exceptions
	fmt.Fprintf(r.writer, "5. OPERATIONAL ERRORS & EXCEPTIONS (%d recorded):\n", len(s.Errors))
	if len(s.Errors) == 0 {
		fmt.Fprintf(r.writer, "   • None (All operations approved cleanly)\n")
	} else {
		for _, errStr := range s.Errors {
			fmt.Fprintf(r.writer, "   • [ERROR/DECLINE] %s\n", errStr)
		}
	}

	// 6. Day 6 Interest Capitalization
	if s.Day == 6 && len(s.InterestCapitalized) > 0 {
		fmt.Fprintf(r.writer, "%s\n", subSep)
		fmt.Fprintf(r.writer, "6. DAY 6 FINAL INTEREST CAPITALIZATION:\n")
		for accID, capAmt := range s.InterestCapitalized {
			fmt.Fprintf(r.writer, "   • Account: %s | Capitalized Total Credit: %s (Exact sum of daily rounded accruals)\n",
				accID, capAmt.Format())
		}
	}

	fmt.Fprintf(r.writer, "%s\n\n", sep)
}
