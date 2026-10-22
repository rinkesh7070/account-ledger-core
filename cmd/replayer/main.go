package main

import (
	"fmt"
	"os"

	"github.com/rinkesh7070/account-ledger-core/internal/engine"
	"github.com/rinkesh7070/account-ledger-core/internal/replayer"
)

func main() {
	fmt.Println("================================================================================")
	fmt.Println("              IN-MEMORY ACCOUNT LEDGER CORE — EVENT STREAM REPLAYER              ")
	fmt.Println("                       6-DAY FINANCIAL AUDIT SIMULATION                         ")
	fmt.Println("================================================================================")
	fmt.Println("Accounts:")
	fmt.Println("  • ACC-001 — AED (precision: 2 decimals, scale: 100 fils), opening balance 0.00")
	fmt.Println("  • ACC-002 — BHD (precision: 3 decimals, scale: 1000 fils), opening balance 0.000")
	fmt.Println("Non-negotiable Rules:")
	fmt.Println("  • Overdraft Fee: AED 25.00 once per day when closing balance is negative")
	fmt.Println("  • Daily Interest: 0.04% per day on positive closing ledger balance")
	fmt.Println("  • Capitalization: Single credit at end of Day 6, exact sum of daily accruals")
	fmt.Println("  • Append-Only: No ledger records are ever mutated or deleted")
	fmt.Println("  • Authorization: Approved only if available balance >= 0 after hold")
	fmt.Println("================================================================================")

	// Policy: AllowForcePost true models real-world payment scheme presentments (Visa/Mastercard)
	// where offline settlements without prior authorization must be debited.
	policy := engine.LedgerPolicy{AllowForcePost: true}
	rep := replayer.NewReplayer(os.Stdout, policy)

	summaries, err := rep.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Replayer fatal error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("================================================================================")
	fmt.Println("                        6-DAY FINAL RECONCILIATION AUDIT                        ")
	fmt.Println("================================================================================")
	fmt.Printf("%-6s | %-16s | %-16s | %-16s | %-12s\n", "Day", "ACC-001 Ledger", "ACC-001 Avail", "ACC-002 Ledger", "Fees Assessed")
	fmt.Println("--------------------------------------------------------------------------------")
	for _, s := range summaries {
		feesCount := fmt.Sprintf("%d fee(s)", len(s.FeesAssessed))
		fmt.Printf("Day %-2d | %16s | %16s | %16s | %-12s\n",
			s.Day,
			s.ClosingBalances["ACC-001"].Format(),
			s.AvailableBalances["ACC-001"].Format(),
			s.ClosingBalances["ACC-002"].Format(),
			feesCount,
		)
	}
	fmt.Println("================================================================================")
	fmt.Println("Replay simulation completed successfully.")
}
