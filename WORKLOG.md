# WORKLOG.md — Account Ledger Core Development Journal

All timestamps recorded in local time (UTC+04:00, Asia/Dubai).

---

### [2026-10-18 11:32:30] — Requirements Analysis & Critical Spec Audit
- Received problem specification for in-memory account ledger core operating across Day 1 through Day 6.
- Audited 8 acceptance criteria:
  - Validated Criterion 1: Day 2 closing ledger balance evaluated at end of Day 5 is AED -370.00.
  - Refused Criterion 2: E7 causes overdraft on Day 2 AND cascades forward to cause an overdraft on Day 4 (and Day 5). Stating "exactly one overdraft fee on Day 2" is incorrect.
  - Validated Criterion 3: Settlement of Auth-A for AED 185.00 against AED 200.00 hold is standard partial capture / under-settlement.
  - Identified ambiguity & scheme rule in Criterion 4: Card scheme presentments without authorization are "Force Posts" which legally cannot be rejected at clearing. Documented trade-off between strict authorization match and ISO 8583 payment scheme rules.
  - Identified contradiction in Criterion 5: On Day 5, account ACC-001 has negative available balance due to E7 and cascading fees. Thus Auth-B cannot be approved; it must be declined per the non-negotiable authorization rule.
  - Refused Criterion 6: Reversal E9 restores principal balance, but in an append-only ledger, assessed overdraft fees persist unless explicitly waived. Declined Auth-B cannot retroactively be approved.
  - Refused Criterion 7: Posting three instalments of BHD 3.334 creates 10.002 BHD (+0.002 BHD / 2 fils out of thin air). Conservation of money mandates fractional remainder distribution (3.334, 3.333, 3.333).
  - Refused Criterion 8: Discarding rounding remainder contradicts the non-negotiable rule requiring capitalized interest to equal exact sum of daily accruals.
- Designed system plan: pure Go in-memory event-sourced core (zero external dependencies) with clean interface abstraction for MySQL relational persistence.

---

### [2026-10-18 11:41:00] — Repository Initialization
- Initialized Git repository on `main` branch.
- Created `go.mod` for `github.com/rinkesh7070/account-ledger-core` (Go 1.25).
- Created `.gitignore` ignoring build artifacts, binary executables, and OS metadata.

---

### [2026-10-18 11:42:15] — Domain Modeling & Exact Minor-Unit Money Arithmetic
- Implemented `internal/domain/money.go`:
  - Fixed-point integer minor units (`int64` fils/cents). Zero floating-point representation.
  - Currencies supported: `AED` (2 decimals, scale factor 100), `BHD` (3 decimals, scale factor 1000).
  - Implemented `Split(parts)` allocating remainders fil-by-fil to preserve conservation of money.
  - Implemented `RoundInterest` for 0.04% daily accrual with half-up rounding.
- Implemented `internal/domain/events.go`:
  - Modeled immutable `Event` records (Credit, Debit, Authorization, Settlement, Reversal, Overdraft Fee, Interest Capitalization).
  - Modeled `AuthorizationHold` lifecycle: `PENDING`, `APPROVED`, `DECLINED`, `SETTLED`, `EXPIRED`.
- Implemented `internal/domain/account.go` and `internal/domain/errors.go`.
- Wrote and verified unit tests in `internal/domain/money_test.go` (`PASS`).

---

### [2026-10-18 11:43:30] — Core Ledger Engine, Interest & Overdraft Fee Subsystems
- Implemented `internal/engine/interest.go`:
  - Daily accrual tracking on positive closing balances.
  - Day 6 capitalization guaranteeing exact sum equality (`sum(daily_accruals)`).
- Implemented `internal/engine/fee.go`:
  - Overdraft fee assessor enforcing AED 25.00 once per day per account.
  - Value dating logic booking fee with `value_date` equal to the day assessed.
- Implemented `internal/engine/ledger.go`:
  - Thread-safe append-only ledger (`sync.RWMutex`).
  - Closing ledger balance computed dynamically: $\sum \{ e.\text{Amount} \mid e.\text{ValueDate} \le d \}$.
  - Available balance computed as ledger balance minus active holds.
  - Settlement under-capture handling (releasing full hold and debiting settled amount).
  - Reversal mechanics booking offsetting credit entries without mutating history.
- Wrote unit tests in `internal/engine/ledger_test.go` verifying Day 2 balance of -AED 370.00, hold approval/decline, and settlements (`PASS`).

---

### [2026-10-18 11:45:50] — Replayer Pipeline & Daily Close Reporter
- Implemented `internal/replayer/reporter.go`:
  - Formatted daily audit reports printing:
    1. Booked events processed
    2. Closing ledger balances & available balances
    3. Fee assessments (amount, booking day, value date, audit reason)
    4. Authorization states (hold amounts, approval/decline status)
    5. Operational errors & exceptions
    6. Day 6 final interest capitalization summary
- Implemented `internal/replayer/replayer.go`:
  - 6-day financial simulation pipeline replaying E1 through E10.
  - Handled backdated E7 arriving on Day 5 and cascading overdraft fees to Day 2, Day 4, and Day 5.
  - Handled E8 Auth-B decline due to negative available balance.
  - Handled E10 BHD 10.000 split across 3 instalments: [3.334, 3.333, 3.333].
  - Handled Day 6 reversal E9 and final interest capitalization.
- Implemented `cmd/replayer/main.go` entrypoint and verified terminal execution (`PASS`).

---

### [2026-10-18 11:47:30] — Full Test Suite & Annotated Failing Design Boundary Test
- Implemented `test/ledger_test.go`:
  - Full end-to-end replay verification across all 6 days.
  - BHD instalment conservation test.
  - Append-only immutability test.
- Implemented `test/design_boundary_test.go`:
  - Created deliberate, staff-level failing test asserting naive Acceptance Criterion 6 ("After E9, all balances and fees return to their pre-E7 values").
  - Extensively inline-annotated why it fails and what architectural boundaries it reveals (immutability of assessed fees and non-resurrection of declined point-of-sale authorizations).

---

### [2026-10-18 11:49:50] — Production MySQL 8.0 Persistence Architecture
- Implemented `db/schema.sql`:
  - InnoDB tables: `accounts`, `ledger_entries`, `authorization_holds`, `daily_closing_snapshots`, `outbox_events`.
  - Monotonic global sequences, compound indices on `(account_id, value_day)`, idempotency keys, and transactional outbox.

---

### [2026-10-18 11:51:50] — Documentation Suite Completion
- Created `NUMBERS.md`: Comprehensive mathematical, economic, and regulatory defense of all chosen constants.
- Created `AMBIGUITIES.md`: Exhaustive analysis of 10 specification edge cases and their resolutions.
- Created `REJECTED.md`: Rigorous refutation of flawed criteria and abandoned engineering approaches.
- Created `ARCHITECTURE.md`: Complete system architecture, concurrency invariants, and MySQL persistence blueprint.
- Created `README.md`: Execution instructions and report interpretation guide.
