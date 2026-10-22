package domain

import (
	"testing"
)

func TestMoney_FromDecimalString(t *testing.T) {
	tests := []struct {
		input    string
		curr     Currency
		expected int64
		format   string
	}{
		{"1200.00", AED, 120000, "AED 1,200.00"},
		{"950.00", AED, 95000, "AED 950.00"},
		{"10.000", BHD, 10000, "BHD 10.000"},
		{"-370.00", AED, -37000, "-AED 370.00"},
		{"0.00", AED, 0, "AED 0.00"},
		{"25.00", AED, 2500, "AED 25.00"},
	}

	for _, tc := range tests {
		m, err := FromDecimalString(tc.input, tc.curr)
		if err != nil {
			t.Fatalf("unexpected error parsing %s: %v", tc.input, err)
		}
		if m.Amount() != tc.expected {
			t.Errorf("input %s: expected amount %d, got %d", tc.input, tc.expected, m.Amount())
		}
		if m.Format() != tc.format {
			t.Errorf("input %s: expected format %s, got %s", tc.input, tc.format, m.Format())
		}
	}
}

func TestMoney_SplitConservation(t *testing.T) {
	// 10.000 BHD split into 3 parts
	m, _ := FromDecimalString("10.000", BHD)
	parts, err := m.Split(3)
	if err != nil {
		t.Fatalf("unexpected error splitting: %v", err)
	}
	if len(parts) != 3 {
		t.Fatalf("expected 3 parts, got %d", len(parts))
	}

	// 10,000 fils / 3 = 3,333 remainder 1 fil
	// Parts must be 3,334 + 3,333 + 3,333 = 10,000 fils exactly
	if parts[0].Amount() != 3334 || parts[1].Amount() != 3333 || parts[2].Amount() != 3333 {
		t.Errorf("expected [3334, 3333, 3333], got [%d, %d, %d]",
			parts[0].Amount(), parts[1].Amount(), parts[2].Amount())
	}

	// Verify sum
	sum := NewMoney(0, BHD)
	for _, p := range parts {
		sum, _ = sum.Add(p)
	}
	if sum.Amount() != 10000 {
		t.Errorf("conservation of money violated: expected 10000 fils, got %d", sum.Amount())
	}
}

func TestMoney_InterestAccrual(t *testing.T) {
	// AED 250.00 closing balance, daily interest 0.04% (0.0004)
	// 250.00 * 0.0004 = 0.1000 AED = 10 fils
	bal1, _ := FromDecimalString("250.00", AED)
	interest1 := RoundInterest(bal1, 0.0004)
	if interest1.Amount() != 10 { // 10 fils = 0.10 AED
		t.Errorf("expected 10 fils interest, got %d", interest1.Amount())
	}

	// AED 650.00 closing balance, daily interest 0.04%
	// 650.00 * 0.0004 = 0.2600 AED = 26 fils
	bal2, _ := FromDecimalString("650.00", AED)
	interest2 := RoundInterest(bal2, 0.0004)
	if interest2.Amount() != 26 {
		t.Errorf("expected 26 fils interest, got %d", interest2.Amount())
	}

	// Negative balance does not accrue interest
	negBal, _ := FromDecimalString("-370.00", AED)
	interestNeg := RoundInterest(negBal, 0.0004)
	if !interestNeg.IsZero() {
		t.Errorf("negative balance must accrue zero interest, got %d", interestNeg.Amount())
	}
}
