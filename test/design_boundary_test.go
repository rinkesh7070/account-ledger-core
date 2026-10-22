package test

import (
	"bytes"
	"testing"

	"github.com/rinkesh7070/account-ledger-core/internal/domain"
	"github.com/rinkesh7070/account-ledger-core/internal/engine"
	"github.com/rinkesh7070/account-ledger-core/internal/replayer"
)

/*
================================================================================
DESIGN BOUNDARY TEST — INLINE ANNOTATION & ARCHITECTURAL DISCLOSURE
================================================================================

WHAT THIS FAILING TEST REVEALS ABOUT THE ARCHITECTURE:

This test codifies the naive expectation articulated in flawed Acceptance Criterion 6:
    "After E9, all balances and fees return to their pre-E7 values."

This test INTENTIONALLY FAILS against our design. It exposes why treating a financial
ledger as a mutable state cache rather than a time-linear event store is fatal:

1. AUDIT IMMUTABILITY OF ASSESSED PENALTIES (FEES DO NOT RETROACTIVELY VANISH):
   When E7 (debit AED 620.00 with value_date Day 2) arrived on Day 5, account ACC-001
   genuinely plunged into negative balance across Day 2, Day 4, and Day 5.
   The overdraft fees (AED 25.00 each) were legally and economically assessed and booked
   as immutable ledger events.
   When E9 arrives on Day 6 reversing E7, it books an offsetting credit of AED 620.00.
   In an append-only ledger, a reversal of principal does NOT delete or erase previously
   assessed fee records. Without a distinct, human-authorized or policy-driven
   FEE_WAIVER event, the fees stand. Erasing them would break append-only immutability
   and violate IFRS / GAAP financial audit compliance.

2. TIME-LINEAR REALITY OF AUTHORIZATION HOLDS (DECLINED AUTHS CANNOT TIME-TRAVEL):
   On Day 5, when Auth-B (AED 90.00) was presented, the account's available balance
   was negative (-AED 335.00). The engine declined Auth-B synchronously.
   A merchant or payment terminal was informed that the transaction was DECLINED.
   Reversing E7 on Day 6 cannot travel backwards in time to magically "approve"
   a card transaction that already failed at the point of sale.

Therefore, this test asserts that fees return to zero and Auth-B is approved.
Because our design correctly protects audit immutability and real-time authorization
integrity, THIS TEST FAILS.
================================================================================
*/

func TestDesignBoundary_NaivePreE7ReversalExpectation(t *testing.T) {
	var buf bytes.Buffer
	policy := engine.LedgerPolicy{AllowForcePost: true}
	rep := replayer.NewReplayer(&buf, policy)

	summaries, err := rep.Run()
	if err != nil {
		t.Fatalf("unexpected simulation error: %v", err)
	}

	// At end of Day 6, evaluate naive criterion 6 expectations:
	day6 := summaries[5]

	// 1. Naive assertion: All assessed overdraft fees were wiped out / deleted
	ledgerEvents := rep.Ledger().GetEvents()
	var feeEventsCount int
	for _, e := range ledgerEvents {
		if e.Type == domain.EventTypeOverdraftFee {
			feeEventsCount++
		}
	}

	t.Logf("[DESIGN BOUNDARY AUDIT] Total Overdraft Fee events booked in ledger: %d", feeEventsCount)
	t.Logf("[DESIGN BOUNDARY AUDIT] Day 6 ACC-001 Ledger Balance: %s", day6.ClosingBalances["ACC-001"].Format())

	// This assertion asserts the flawed criterion: "fees return to pre-E7 values (0 fees)"
	if feeEventsCount != 0 {
		t.Errorf(`
[INTENTIONAL ARCHITECTURAL TEST FAILURE]
Flawed Acceptance Criterion 6 Refuted!
Expected 0 fees after reversal E9, but found %d fee entries in the append-only ledger!
Reason: Ledger records are immutable. Reversing principal does NOT delete legally assessed overdraft fees.`,
			feeEventsCount,
		)
	}

	// 2. Naive assertion: Auth-B was retroactively approved
	holds := rep.Ledger().GetAllHolds("ACC-001")
	var authBStatus domain.AuthStatus
	for _, h := range holds {
		if h.AuthID == "Auth-B" {
			authBStatus = h.Status
		}
	}

	if authBStatus != domain.AuthStatusApproved {
		t.Errorf(`
[INTENTIONAL ARCHITECTURAL TEST FAILURE]
Flawed Acceptance Criterion 5/6 Refuted!
Expected Auth-B to be APPROVED after E7 reversal, but status is %s!
Reason: Authorizations cannot be approved retroactively. On Day 5, available funds were negative.`,
			authBStatus,
		)
	}
}
