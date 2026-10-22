# AMBIGUITIES.md — Detailed Analysis and Resolutions of Specification Gaps

In financial ledger engineering, underspecified edge cases, conflicting rules, and temporal paradoxes frequently arise. This document catalogs every ambiguity discovered in the specification and details the exact domain, accounting, and technical principles applied to resolve them.

---

### Ambiguity 1: Replay Sequence vs. Booking Day for Event E10
- **The Conflict:** In the problem statement, the event list reads:
  ```
  E7  — Day 5 — DEBIT — ACC-001 AED 620.00 — value_date Day 2
  E8  — Day 5 — AUTHORIZATION — ACC-001 Auth-B hold AED 90.00 — value_date Day 5
  E9  — Day 6 — REVERSAL — ACC-001 reverses E7 — value_date Day 2
  E10 — Day 5 — CREDIT — ACC-002 BHD 10.000, posted as three equal instalments — value_date Day 5
  ```
  The prompt states *"Event stream, replayed in this order:"* and lists E10 after E9 (Day 6), yet E10's booked day is explicitly labeled **Day 5**.
- **Analysis:** In distributed event streaming, events may arrive out of order relative to their booking timestamps. There are two interpretations:
  1. *Stream-Order Execution:* Replay strictly follows the linear text order (E1 through E10), processing E10 after E9 on Day 6.
  2. *Calendar-Day Batch Execution:* All Day 5 events (E7, E8, E10) are ingested and evaluated during Day 5 before advancing to Day 6.
- **Resolution:** Because E10 targets account `ACC-002` (which is completely isolated from `ACC-001` in currency and balance), the order of E10 relative to E9 has zero mathematical impact on either account's ledger balance or fee assessment. However, to honor the Day 5 booking date and daily close report format, our engine ingests E10 as part of Day 5's processing.

---

### Ambiguity 2: Settlement Without Preceding Authorization (E6 Auth-Z)
- **The Conflict:** Event E6 settles `Auth-Z` for AED 180.00 on Day 4, but `Auth-Z` was never preceded by an authorization event. Flawed Acceptance Criterion 4 asserts:
  > *"Any settlement referencing an authorization ID not present in the ledger must be rejected and the funds must not leave the account."*
- **Analysis:** In modern payment networks (Visa, Mastercard, ISO 8583), an offline settlement presented without a prior authorization is known as a **Force Post** or **Offline Transaction** (common in airline in-flight purchases, transit floor limits, or when authorizations expire before merchant settlement clearing). Under card scheme rules, the issuing bank **cannot reject** a valid financial presentment clearing file. Funds *must* leave the account, even if it forces the account into overdraft.
- **Resolution:** We introduced a configurable engine policy: `LedgerPolicy{AllowForcePost: bool}`.
  - If `AllowForcePost == false`: E6 is rejected as an unmatched authorization error (`ErrAuthorizationNotFound`), funds do not leave the account.
  - If `AllowForcePost == true` (default in our replayer): E6 is honored as a standard card scheme Force Post, debiting AED 180.00 directly to the ledger.
  We document both paths and evaluate their downstream impacts.

---

### Ambiguity 3: Backdated Debit & Cascading Overdraft Fee Booking
- **The Conflict:** E7 arrives on Day 5 as a debit of AED 620.00 with `value_date = Day 2`.
  The non-negotiable rule specifies:
  > *"Overdraft fee: AED 25.00, assessed once per day per account when that day's closing ledger balance (all entries with value_date <= that day) is negative. Booked with value_date equal to the day assessed."*
  When Day 2 is re-evaluated at the end of Day 5, the closing balance is negative (AED -370.00). When is the resulting overdraft fee booked?
- **Analysis:** Does the fee get booked with `value_date = Day 2` (the day being assessed) or `value_date = Day 5` (the day the overdraft was discovered)?
- **Resolution:** The spec explicitly commands: *"Booked with value_date equal to the day assessed."*
  The day assessed is Day 2. Therefore, the fee event is booked with:
  - `BookingDay = Day 5` (when the audit engine ran)
  - `ValueDay = Day 2` (the day assessed)
  Because this fee has `value_date = Day 2`, it reduces the historical balance of Day 2 to AED -395.00. This deduction cascades forward:
  - Day 3 balance: $+5.00$ AED (positive).
  - Day 4 balance: $+5.00 - 185.00 - 180.00 = -360.00$ AED (negative!).
  Consequently, Day 4's closing balance drops below zero, triggering a second overdraft fee for Day 4.

---

### Ambiguity 4: Partial Settlement and Hold Release Mechanics (Auth-A)
- **The Conflict:** On Day 2, Auth-A holds AED 200.00. On Day 4, Event E5 settles Auth-A for AED 185.00 (under-capture / partial settlement).
- **Analysis:** Does the remaining AED 15.00 hold remain active against available balance, or is the entire AED 200.00 hold released?
- **Resolution:** In banking and card scheme clearing (Visa Base II / Mastercard IPM), a settlement presentment matching an authorization releases the entire hold unless an explicit "partial settlement / split shipment" indicator is provided. Retaining orphan hold residuals after final merchant clearing would unnecessarily restrict customer funds. Therefore, upon settlement of E5, the full AED 200.00 hold is released, and AED 185.00 is posted as a financial debit.

---

### Ambiguity 5: Reversal Scope on Assessed Fees and Historical Inquiries
- **The Conflict:** E9 reverses E7 on Day 6 with `value_date = Day 2`. Flawed Acceptance Criterion 6 claims:
  > *"After E9, all balances and fees return to their pre-E7 values."*
- **Analysis:** Does reversing the underlying transaction magically erase or invalidate the overdraft fees assessed on Day 2, Day 4, and Day 5?
- **Resolution:** In an append-only ledger, **records are never mutated or deleted**. An overdraft fee assessed when an account was in negative balance is an independent legal and accounting event. Unless a distinct `FEE_WAIVER` event is explicitly booked by a human officer or business rule, assessed fees remain in the ledger. Reversing E7 credits the principal back (+AED 620.00), but the fees remain.

---

### Ambiguity 6: Order of Operations at Daily Close
- **The Conflict:** At the end of each day, the engine must evaluate both interest accruals and overdraft fees. Which runs first?
- **Analysis:** If an overdraft fee is assessed and booked with value date equal to that day, does it reduce the balance before or after interest is calculated?
- **Resolution:**
  - Interest accrues **only on positive balances** ($0.04\%$ on closing balance $> 0$).
  - Overdraft fees are assessed **only on negative balances** (closing balance $< 0$).
  Because the conditions are mutually exclusive, there is zero ambiguity: an account cannot be simultaneously positive and negative. If negative, fee is assessed and interest is zero. If positive, interest accrues and fee is zero.

---

### Ambiguity 7: BHD Instalment Remainder Allocation
- **The Conflict:** Event E10 credits BHD 10.000 to ACC-002, posted as three equal instalments.
  $10{,}000 \text{ fils} / 3 = 3{,}333.333\dots \text{ fils}$.
- **Analysis:** How should the indivisible 1 fil remainder be distributed?
- **Resolution:** The three instalments must sum to exactly 10,000 fils to maintain conservation of money. Distributing the remainder to the first instalment yields:
  - Instalment 1: BHD 3.334 ($3{,}334$ fils)
  - Instalment 2: BHD 3.333 ($3{,}333$ fils)
  - Instalment 3: BHD 3.333 ($3{,}333$ fils)
  $$\text{Sum} = 3{,}334 + 3{,}333 + 3{,}333 = 10{,}000 \text{ fils} \equiv \text{BHD } 10.000$$

---

### Ambiguity 8: Lifespan of Auth-B
- **The Conflict:** Note states: *"Auth-B is never settled inside the window."*
- **Analysis:** What is the operational state of Auth-B throughout Days 5 and 6?
- **Resolution:** When Auth-B is requested on Day 5 (AED 90.00), ACC-001 has a negative available balance due to E7 and cascading fees. Per the non-negotiable authorization rule, Auth-B is **declined immediately** at the time of request. Because it was declined, no active hold was ever created, and settlement is impossible.

---

### Ambiguity 9: Available Balance vs. Accrued Interest
- **The Conflict:** Does available balance include daily accrued interest before it is capitalized at Day 6?
- **Analysis:** In banking practice, accrued interest is an off-ledger or uncapitalized memorandum asset.
- **Resolution:** Accrued interest does not become spendable cash until it is formalized and posted to the ledger as a single credit event at the end of Day 6. Therefore, available balance excludes uncapitalized interest accruals.

---

### Ambiguity 10: Multi-Currency Ledger Isolation
- **The Conflict:** ACC-001 is denominated in AED; ACC-002 is denominated in BHD.
- **Analysis:** Can balances or fees cross currency boundaries?
- **Resolution:** Complete isolation. The ledger prevents any cross-currency arithmetic (`ErrCurrencyMismatch`). Overdraft fees on ACC-001 (AED 25.00) have no relationship to ACC-002 (BHD).
