package engine

import (
	"testing"

	"github.com/rinkesh7070/account-ledger-core/internal/domain"
)

func setupTestLedger() (*Ledger, *InterestCalculator, *OverdraftFeeAssessor) {
	feeAmount, _ := domain.FromDecimalString("25.00", domain.AED)
	feeAssessor := NewOverdraftFeeAssessor(feeAmount)
	interestCalc := NewInterestCalculator(0.0004) // 0.04% per day
	ledger := NewLedger(feeAssessor, interestCalc, LedgerPolicy{AllowForcePost: true})

	ledger.RegisterAccount(domain.NewAccount("ACC-001", domain.AED))
	ledger.RegisterAccount(domain.NewAccount("ACC-002", domain.BHD))

	return ledger, interestCalc, feeAssessor
}

func TestLedger_Criterion1_Day2BalanceAtEndOfDay5(t *testing.T) {
	// Acceptance Criterion 1:
	// "The Day 2 closing ledger balance, evaluated at end of Day 5 and before any fee is assessed, is AED −370.00."
	ledger, _, _ := setupTestLedger()

	e1Amt, _ := domain.FromDecimalString("1200.00", domain.AED)
	ledger.PostCredit("E1", "ACC-001", e1Amt, 1, 1, "E1 Credit")

	e2Amt, _ := domain.FromDecimalString("950.00", domain.AED)
	ledger.PostDebit("E2", "ACC-001", e2Amt, 1, 1, "E2 Debit")

	authAAmt, _ := domain.FromDecimalString("200.00", domain.AED)
	ledger.RequestAuthorization("Auth-A", "ACC-001", authAAmt, 2, 2)

	e7Amt, _ := domain.FromDecimalString("620.00", domain.AED)
	ledger.PostDebit("E7", "ACC-001", e7Amt, 5, 2, "E7 Backdated Debit")

	// Calculate Day 2 closing ledger balance before any fees are assessed
	bal, err := ledger.CalculateClosingLedgerBalance("ACC-001", 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected, _ := domain.FromDecimalString("-370.00", domain.AED)
	if bal.Amount() != expected.Amount() {
		t.Fatalf("expected Day 2 balance %s, got %s", expected.Format(), bal.Format())
	}
}

func TestLedger_AuthorizationApprovalAndDecline(t *testing.T) {
	ledger, _, _ := setupTestLedger()

	e1Amt, _ := domain.FromDecimalString("1200.00", domain.AED)
	ledger.PostCredit("E1", "ACC-001", e1Amt, 1, 1, "E1")

	e2Amt, _ := domain.FromDecimalString("950.00", domain.AED)
	ledger.PostDebit("E2", "ACC-001", e2Amt, 1, 1, "E2")

	// Balance = 250.00 AED
	// Auth-A for 200.00 AED -> approved (available 50.00)
	authAmt, _ := domain.FromDecimalString("200.00", domain.AED)
	hold, err := ledger.RequestAuthorization("Auth-A", "ACC-001", authAmt, 2, 2)
	if err != nil {
		t.Fatalf("expected Auth-A approved, got error: %v", err)
	}
	if hold.Status != domain.AuthStatusApproved {
		t.Fatalf("expected status APPROVED, got %s", hold.Status)
	}

	// Ledger balance must remain 250.00 AED
	bal, _ := ledger.CalculateClosingLedgerBalance("ACC-001", 2)
	if bal.Amount() != 25000 {
		t.Errorf("ledger balance should remain 250.00 AED, got %s", bal.Format())
	}

	// Available balance must be 50.00 AED
	avail, _ := ledger.GetAvailableBalance("ACC-001")
	if avail.Amount() != 5000 {
		t.Errorf("available balance should be 50.00 AED, got %s", avail.Format())
	}

	// Now try hold for 60.00 AED -> must decline (insufficient funds)
	excessAmt, _ := domain.FromDecimalString("60.00", domain.AED)
	_, err = ledger.RequestAuthorization("Auth-Excess", "ACC-001", excessAmt, 2, 2)
	if err == nil {
		t.Fatalf("expected authorization decline, but succeeded")
	}
}

func TestLedger_SettlementUnderCapture(t *testing.T) {
	ledger, _, _ := setupTestLedger()

	e1Amt, _ := domain.FromDecimalString("1200.00", domain.AED)
	ledger.PostCredit("E1", "ACC-001", e1Amt, 1, 1, "E1")

	// Auth for 200.00 AED
	authAmt, _ := domain.FromDecimalString("200.00", domain.AED)
	ledger.RequestAuthorization("Auth-A", "ACC-001", authAmt, 2, 2)

	// Settlement for 185.00 AED on Day 4
	settleAmt, _ := domain.FromDecimalString("185.00", domain.AED)
	_, err := ledger.SettleAuthorization("E5", "Auth-A", "ACC-001", settleAmt, 4, 4)
	if err != nil {
		t.Fatalf("expected settlement success: %v", err)
	}

	// Active holds should be 0
	activeHolds := ledger.GetActiveHolds("ACC-001")
	if len(activeHolds) != 0 {
		t.Fatalf("expected 0 active holds, got %d", len(activeHolds))
	}

	// Ledger balance should be 1200.00 - 185.00 = 1015.00 AED
	bal, _ := ledger.CalculateClosingLedgerBalance("ACC-001", 4)
	if bal.Amount() != 101500 {
		t.Fatalf("expected balance 1015.00 AED, got %s", bal.Format())
	}
}
