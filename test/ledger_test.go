package test

import (
	"bytes"
	"testing"

	"github.com/rinkesh7070/account-ledger-core/internal/domain"
	"github.com/rinkesh7070/account-ledger-core/internal/engine"
	"github.com/rinkesh7070/account-ledger-core/internal/replayer"
)

// TestLedger_FullReplaySuite tests the 6-day replay simulation against all non-negotiable rules.
func TestLedger_FullReplaySuite(t *testing.T) {
	var buf bytes.Buffer
	policy := engine.LedgerPolicy{AllowForcePost: true}
	rep := replayer.NewReplayer(&buf, policy)

	summaries, err := rep.Run()
	if err != nil {
		t.Fatalf("replayer execution failed: %v", err)
	}

	if len(summaries) != 6 {
		t.Fatalf("expected 6 daily summaries, got %d", len(summaries))
	}

	// Day 1 Checks:
	// E1 (Credit 1200), E2 (Debit 950) -> Closing balance: 250.00 AED
	day1 := summaries[0]
	if day1.ClosingBalances["ACC-001"].Amount() != 25000 {
		t.Errorf("Day 1 closing balance expected 250.00 AED, got %s", day1.ClosingBalances["ACC-001"].Format())
	}
	if len(day1.FeesAssessed) != 0 {
		t.Errorf("Day 1 should have no fees, got %d", len(day1.FeesAssessed))
	}
	// Day 1 interest: 250.00 * 0.0004 = 0.10 AED (10 fils)
	if day1.InterestAccrued["ACC-001"].Amount() != 10 {
		t.Errorf("Day 1 interest expected 0.10 AED, got %s", day1.InterestAccrued["ACC-001"].Format())
	}

	// Day 2 Checks:
	// Auth-A hold 200.00 AED -> Approved.
	// Ledger balance remains 250.00 AED. Available balance drops to 50.00 AED.
	day2 := summaries[1]
	if day2.ClosingBalances["ACC-001"].Amount() != 25000 {
		t.Errorf("Day 2 closing balance expected 250.00 AED, got %s", day2.ClosingBalances["ACC-001"].Format())
	}
	if day2.AvailableBalances["ACC-001"].Amount() != 5000 {
		t.Errorf("Day 2 available balance expected 50.00 AED, got %s", day2.AvailableBalances["ACC-001"].Format())
	}

	// Day 3 Checks:
	// E4 Credit 400.00 AED -> Ledger balance = 650.00 AED. Available = 450.00 AED.
	day3 := summaries[2]
	if day3.ClosingBalances["ACC-001"].Amount() != 65000 {
		t.Errorf("Day 3 closing balance expected 650.00 AED, got %s", day3.ClosingBalances["ACC-001"].Format())
	}
	if day3.AvailableBalances["ACC-001"].Amount() != 45000 {
		t.Errorf("Day 3 available balance expected 450.00 AED, got %s", day3.AvailableBalances["ACC-001"].Format())
	}
	// Day 3 interest: 650.00 * 0.0004 = 0.26 AED (26 fils)
	if day3.InterestAccrued["ACC-001"].Amount() != 26 {
		t.Errorf("Day 3 interest expected 0.26 AED, got %s", day3.InterestAccrued["ACC-001"].Format())
	}

	// Day 4 Checks:
	// E5 Auth-A settles for 185.00 AED (hold 200 released).
	// E6 Auth-Z settles for 180.00 AED (force post).
	// Balance = 650 - 185 - 180 = 285.00 AED.
	day4 := summaries[3]
	if day4.ClosingBalances["ACC-001"].Amount() != 28500 {
		t.Errorf("Day 4 closing balance expected 285.00 AED, got %s", day4.ClosingBalances["ACC-001"].Format())
	}

	// Day 5 Checks:
	// E7 Backdated debit 620.00 AED to Day 2!
	// Triggers cascading overdraft fees on Day 2, Day 4, Day 5.
	// E8 Auth-B requested for 90.00 AED -> must be DECLINED!
	// E10 BHD 10.000 credited in 3 instalments: 3.334, 3.333, 3.333 -> ACC-002 closing balance = 10.000 BHD.
	day5 := summaries[4]
	if len(day5.FeesAssessed) != 3 {
		t.Errorf("Day 5 should record 3 cascading overdraft fees, got %d", len(day5.FeesAssessed))
	}
	// Verify Auth-B was declined
	foundDeclinedAuthB := false
	for _, errStr := range day5.Errors {
		if bytes.Contains([]byte(errStr), []byte("Auth-B DECLINED")) {
			foundDeclinedAuthB = true
			break
		}
	}
	if !foundDeclinedAuthB {
		t.Errorf("expected Auth-B to be declined on Day 5 due to negative balance")
	}
	// Verify ACC-002 balance is exactly 10.000 BHD
	if day5.ClosingBalances["ACC-002"].Amount() != 10000 {
		t.Errorf("ACC-002 balance expected 10.000 BHD (10000 fils), got %s", day5.ClosingBalances["ACC-002"].Format())
	}

	// Day 6 Checks:
	// E9 Reversal of E7 posted.
	// Interest capitalized: sum of daily accruals credited as single credit.
	day6 := summaries[5]
	acc1Cap, ok := day6.InterestCapitalized["ACC-001"]
	if !ok || !acc1Cap.IsPositive() {
		t.Errorf("expected Day 6 interest capitalization for ACC-001, got %s", acc1Cap.Format())
	}
	acc2Cap, ok := day6.InterestCapitalized["ACC-002"]
	if !ok || !acc2Cap.IsPositive() {
		t.Errorf("expected Day 6 interest capitalization for ACC-002, got %s", acc2Cap.Format())
	}
}

// TestLedger_InstalmentConservation verifies that splitting 10.000 BHD preserves exact total.
func TestLedger_InstalmentConservation(t *testing.T) {
	m, err := domain.FromDecimalString("10.000", domain.BHD)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	parts, err := m.Split(3)
	if err != nil {
		t.Fatalf("unexpected split error: %v", err)
	}

	// Must be 3.334, 3.333, 3.333
	if parts[0].Amount() != 3334 || parts[1].Amount() != 3333 || parts[2].Amount() != 3333 {
		t.Errorf("instalment breakdown invalid: %v", parts)
	}

	// Total must sum exactly to 10.000 BHD (10,000 fils)
	sum := domain.NewMoney(0, domain.BHD)
	for _, p := range parts {
		sum, _ = sum.Add(p)
	}
	if sum.Amount() != 10000 {
		t.Fatalf("money creation/loss detected: expected 10000 fils, got %d", sum.Amount())
	}
}

// TestLedger_AppendOnlyImmutability verifies that event history cannot be mutated or deleted.
func TestLedger_AppendOnlyImmutability(t *testing.T) {
	feeAmount, _ := domain.FromDecimalString("25.00", domain.AED)
	feeAssessor := engine.NewOverdraftFeeAssessor(feeAmount)
	interestCalc := engine.NewInterestCalculator(0.0004)
	ledger := engine.NewLedger(feeAssessor, interestCalc, engine.LedgerPolicy{})

	ledger.RegisterAccount(domain.NewAccount("ACC-001", domain.AED))
	amt, _ := domain.FromDecimalString("500.00", domain.AED)
	e1, _ := ledger.PostCredit("E1", "ACC-001", amt, 1, 1, "Credit 1")

	events1 := ledger.GetEvents()
	if len(events1) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events1))
	}

	// Post reversal
	_, err := ledger.ReverseTransaction("REV-1", "E1", 2, 1)
	if err != nil {
		t.Fatalf("reversal failed: %v", err)
	}

	events2 := ledger.GetEvents()
	if len(events2) != 2 {
		t.Fatalf("expected 2 events in append-only log, got %d", len(events2))
	}

	// Original event E1 must remain completely intact
	if events2[0].ID != e1.ID || events2[0].Amount.Amount() != e1.Amount.Amount() {
		t.Fatalf("original event was mutated in append-only ledger!")
	}
}
