# REJECTED.md — Refused Acceptance Criteria & Abandoned Engineering Approaches

This document fulfills two critical requirements:
1. Formally identifies and refutes every incorrect acceptance criterion provided in the specification with mathematical and domain proof.
2. Details engineering architectures and technical approaches considered and intentionally abandoned during system design.

---

## Part 1: Refused Acceptance Criteria

### 1. Refusal of Criterion: *"E7 causes exactly one overdraft fee to be assessed, on Day 2."*
- **Verdict:** **REFUSED (REJECTED)**
- **Mathematical & Temporal Proof:**
  - On Day 5, E7 arrives as a debit of AED 620.00 with `value_date = Day 2`.
  - Re-evaluating entries with `value_date <= Day 2`:
    $$\text{E1 (+1,200.00)} - \text{E2 (950.00)} - \text{E7 (620.00)} = \text{AED } -370.00$$
  - Because Day 2's closing balance is negative, an Overdraft Fee of AED 25.00 is assessed and booked with `value_date = Day 2`.
  - Day 2's effective balance becomes $-370.00 - 25.00 = \text{AED } -395.00$.
  - Now observe how this cascades forward:
    - **Day 3 Closing Balance:**
      $$-395.00 + \text{E4 (+400.00)} = +\text{AED } 5.00 \quad (\text{Positive})$$
    - **Day 4 Closing Balance:**
      $$+5.00 - \text{E5 (185.00)} - \text{E6 (180.00)} = \text{AED } -360.00 \quad (\text{NEGATIVE!})$$
      *(Even if E6 were rejected, balance would be $+5.00 - 185.00 = \text{AED } -180.00$, which is still strictly negative!)*
  - Because Day 4 closing balance drops below zero, the non-negotiable overdraft rule mandates an assessment:
    > *"assessed once per day per account when that day's closing ledger balance is negative."*
  - Therefore, **Day 4 ALSO triggers an overdraft fee of AED 25.00**.
  - On Day 5, the closing balance remains negative, triggering another overdraft fee for Day 5.
  - **Conclusion:** Stating that E7 causes *exactly one* overdraft fee on Day 2 is demonstrably false. Backdating a debit causes a multi-day fee cascade.

---

### 2. Refusal of Criterion: *"The three BHD instalments in E10 must each be BHD 3.334."*
- **Verdict:** **REFUSED (REJECTED)**
- **Mathematical Proof:**
  - Event E10 specifies a total inbound credit of **BHD 10.000** ($10{,}000$ fils).
  - If three instalments of BHD 3.334 are credited:
    $$3.334 \times 3 = \text{BHD } 10.002 = 10{,}002 \text{ fils}$$
  - Crediting 10.002 BHD when the source deposit was 10.000 BHD creates **0.002 BHD (2 fils) out of thin air**.
  - In financial accounting, creating money without an offsetting debit violates double-entry conservation of money.
  - **Correct Implementation:** Minor units ($10{,}000$ fils) divided by 3 yield a quotient of $3{,}333$ fils and a remainder of $1$ fil. Distributing the indivisible remainder across instalments yields:
    $$\text{Instalment 1: BHD } 3.334 \quad | \quad \text{Instalment 2: BHD } 3.333 \quad | \quad \text{Instalment 3: BHD } 3.333$$
    $$\text{Sum} = 3{,}334 + 3{,}333 + 3{,}333 = 10{,}000 \text{ fils} \equiv \text{BHD } 10.000$$

---

### 3. Refusal of Criterion: *"After E9, all balances and fees return to their pre-E7 values."*
- **Verdict:** **REFUSED (REJECTED)**
- **Domain & Architectural Proof:**
  - **Immutability of Assessed Fees:** When E7 was posted, ACC-001 was legitimately in an unarranged overdraft state. The fees assessed on Days 2, 4, and 5 were valid legal penalties assessed under contract terms. In an append-only ledger (*"The ledger is append-only. No event record is ever mutated or deleted"*), posting a reversal (E9) creates an offsetting credit for the principal (+AED 620.00). It does **not** delete previously booked fee events. Without a distinct `FEE_WAIVER` event, assessed fees permanently stand.
  - **Irreversibility of Point-in-Time Authorizations:** On Day 5, Auth-B requested an AED 90.00 hold and was **declined** because available balance was negative. Reversing E7 on Day 6 cannot travel backwards in time to approve an authorization that already failed at the point of sale.

---

### 4. Refusal of Criterion: *"If the rounded daily interest accruals do not sum to the capitalized total, the remainder is discarded."*
- **Verdict:** **REFUSED (REJECTED)**
- **Domain & Contractual Proof:**
  - This criterion directly contradicts the prompt's non-negotiable rule:
    > *"The rounded daily accruals must sum exactly to the capitalized total."*
  - Discarding remainders introduces a discrepancy between the daily interest memorandum accruals and the capitalized ledger credit.
  - **Correct Implementation:** Capitalization is computed by summing the exact daily rounded accruals:
    $$\text{CapitalizedCredit} \equiv \sum_{d=1}^{6} \text{DailyAccrual}(d)$$
  - This ensures perfect mathematical identity without discarding any fractional currency units.

---

### 5. Refusal of Criterion: *"Any settlement referencing an authorization ID not present in the ledger must be rejected and the funds must not leave the account."*
- **Verdict:** **REFUSED (REJECTED AS GENERAL RULE)**
- **Card Scheme & Payment Network Proof:**
  - In closed-loop systems, rejecting unknown authorizations is valid. However, in core banking and card scheme clearing (Visa/Mastercard/ISO 8583), a settlement without a prior authorization is a **Force Post / Offline Transaction**.
  - Under card network operating regulations, the issuer is legally bound to honor presentments submitted through clearing files. Rejecting a presentment because no prior authorization was found breaches scheme rules and results in compliance fines. The funds *must* leave the account, even if it forces an overdraft.

---

### 6. Refusal of Premise: *"If Auth-B is approved, its hold reduces available balance but not ledger balance."*
- **Verdict:** **REFUSED (PREMISE FALSE)**
- **Proof:** While the statement accurately defines how active holds operate in the abstract, **Auth-B cannot be approved**. When E8 is requested on Day 5, the available balance of ACC-001 is already negative (-AED 335.00) due to E7 and cascading overdraft fees. Per the non-negotiable rule, authorizations are approved *only* if available balance remains $\ge 0$. Auth-B must be declined.

---

## Part 2: Engineering Approaches Abandoned Mid-Build

### 1. IEEE 754 Floating-Point Arithmetic (`float64`)
- **Considered Approach:** Using standard `float64` for amounts (e.g. `1200.00`, `0.0004`).
- **Why Abandoned:** Binary floating-point cannot accurately represent decimal fractions. `0.1 + 0.2 = 0.30000000000000004`. Over thousands of daily recalculations, floating-point drift creates un-reconcilable fractional pennies.
- **Adopted Alternative:** Exact integer minor units (`int64` fils/cents) with explicit scale factors (100 for AED, 1000 for BHD).

### 2. In-Place Balance Mutation (Stateful Snapshot Table)
- **Considered Approach:** Maintaining a single mutable `balance` field per account and updating it with `UPDATE accounts SET balance = balance + amount`.
- **Why Abandoned:** When backdated events (like E7 with `value_date = Day 2` arriving on Day 5) occur, mutable balances cannot reconstruct what the balance was on Day 2 or Day 4 without re-running a full database audit log.
- **Adopted Alternative:** Pure append-only event sourcing (`[]domain.Event`). Every point-in-time closing balance is dynamically computed by summing entries where `value_date <= target_day`.

### 3. Retroactive Deletion of Overdraft Fees on Reversal
- **Considered Approach:** When E9 reverses E7, scan the event log and delete the overdraft fees assessed on Days 2, 4, and 5.
- **Why Abandoned:** Strictly violates the non-negotiable rule: *"The ledger is append-only. No event record is ever mutated or deleted."* In banking compliance (SOX, IFRS), deleting audit events is criminal fraud.
- **Adopted Alternative:** Assessed fee events remain permanently in the ledger log.
