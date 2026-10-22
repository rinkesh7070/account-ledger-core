# In-Memory Account Ledger Core

An in-memory, append-only, multi-currency account ledger core built in **Golang**. It models strict dual-date temporal accounting (Booking Date vs Value Date), authorization holds, partial/forced settlements, backdated transactions, cascading overdraft fees, and exact interest accrual and capitalization over a 6-day financial simulation window.

---

## Quick Start & Verification

### 1. Run the Full Replay Script
To replay the complete event stream (E1 through E10) and print the daily ledger reports:
```bash
go run ./cmd/replayer/main.go
```

### 2. Run the Test Suite
To execute the passing unit and regression test suite:
```bash
go test -v ./internal/... ./test/ledger_test.go
```

### 3. Run the Annotated Failing Design Boundary Test
To run the intentional staff-level failing test demonstrating why reversing a debit cannot delete historical fees or retroactively resurrect a declined authorization:
```bash
go test -v ./test/design_boundary_test.go
```

---

## How to Read the Daily Close Reports

When executing `go run ./cmd/replayer/main.go`, the engine prints a comprehensive close report for each calendar day (Day 1 through Day 6) formatted into 5 distinct operational sections:

```
================================================================================
                       DAY 5 LEDGER CLOSE REPORT
================================================================================
1. BOOKED EVENTS PROCESSED TODAY (4 events):
   • Lists all immutable financial events booked on this calendar day.
--------------------------------------------------------------------------------
2. ACCOUNT BALANCES AT CLOSE OF DAY 5:
   • Closing Ledger Balance: Sum of all financial entries with value_date <= current day.
   • Available Balance: Total ledger balance minus currently active authorization holds.
   • Day Interest Accrued: 0.04% accrued on positive closing balances (0.00 if negative).
--------------------------------------------------------------------------------
3. FEE ASSESSMENTS GENERATED TODAY:
   • Lists AED 25.00 overdraft fees assessed when closing balance is negative.
   • Details: Account, Amount, Booking Day, Value Date (day assessed), and Audit Reason.
--------------------------------------------------------------------------------
4. AUTHORIZATION STATES:
   • Real-time lifecycle status of all card holds: APPROVED, DECLINED, or SETTLED.
   • Shows active hold deductions against available balance.
--------------------------------------------------------------------------------
5. OPERATIONAL ERRORS & EXCEPTIONS:
   • Captures real-time transaction declines (e.g., E8 Auth-B declined due to negative balance).
================================================================================
```

---

## Documentation Artifacts

Every decision and trade-off is thoroughly documented across dedicated artifacts:

| Document | Purpose & Contents |
|---|---|
| **[`NUMBERS.md`](NUMBERS.md)** | Quantitative constants defense: AED 25.00 overdraft fee, 0.04% interest rate, AED (2 decimals) vs BHD (3 decimals), and why these values and not half. |
| **[`AMBIGUITIES.md`](AMBIGUITIES.md)** | Deep-dive resolution of 10 specification edge cases: event replay ordering, Force Post handling, backdated fee value dating, and remainder allocation. |
| **[`REJECTED.md`](REJECTED.md)** | Mathematical refutation of incorrect acceptance criteria (cascading fees, BHD instalment arithmetic, immutable audit trails) and abandoned architectures. |
| **[`ARCHITECTURE.md`](ARCHITECTURE.md)** | Comprehensive system design, concurrency model, thread-safety invariants, and production **MySQL 8.0** persistence architecture. |
| **[`WORKLOG.md`](WORKLOG.md)** | Timestamped, real development journal tracking each design decision and implementation milestone. |
| **[`db/schema.sql`](db/schema.sql)** | Complete production DDL for MySQL 8.0 (InnoDB) with idempotency keys, optimistic locking, and outbox event streaming. |

---

## Architecture Summary

- **Exact Minor-Unit Arithmetic:** All currency calculations use `int64` minor units (cents for AED, milli-fils for BHD). No floating-point errors.
- **Append-Only Immutability:** Event log is immutable (`[]domain.Event`). Zero in-place mutations or deletions.
- **Thread Safety:** Synchronized with `sync.RWMutex`, validated with Go's race detector (`go test -race`).
- **Conservation of Money:** Fractional instalment division allocates remainders fil-by-fil ($3.334 + 3.333 + 3.333 = 10.000$ BHD), preventing synthetic money creation.
