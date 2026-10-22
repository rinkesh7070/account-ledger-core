# ARCHITECTURE.md — System Architecture, Concurrency & Trade-offs Document

## 1. Executive Summary & Core Design Philosophy

The `account-ledger-core` is an append-only, event-sourced financial ledger built to uphold absolute mathematical correctness, zero data loss, and complete audit reproducibility across complex multi-currency lifecycles.

The engine enforces five fundamental banking invariants:
1. **Conservation of Money:** $\sum \text{Credits} - \sum \text{Debits} \equiv \Delta \text{Balance}$. No rounding or remainder can create or destroy currency units.
2. **Strict Append-Only Immutability:** Financial events are immutable facts. Records are never updated in place or deleted. Reversals are recorded as new offsetting events.
3. **Temporal Dual-Date Accounting:** Decoupling **Booking Date** (the physical wall-clock/simulation day an entry was registered) from **Value Date** (the effective accounting date from which interest and overdraft balances are calculated).
4. **Point-in-Time Authorization Finality:** Authorizations reserve available funds against real-time state. Authorizations cannot be retroactively approved after having been declined.
5. **Exact Minor-Unit Integer Arithmetic:** Absolute elimination of IEEE 754 floating-point drift by representing all currency amounts in integer minor units (fils/cents).

---

## 2. In-Memory Core Architecture (Golang)

### 2.1 Domain Layer & Numeric Representation
- **Minor Units (`int64`):**
  - AED: scale factor $10^2 = 100$ ($1 \text{ AED} = 100 \text{ fils}$).
  - BHD: scale factor $10^3 = 1{,}000$ ($1 \text{ BHD} = 1{,}000 \text{ fils}$).
  - Calculations execute exclusively on `int64` integers.
  - Remainder distribution (`Split(n)`) redistributes remainders fil-by-fil, guaranteeing that $10.000$ BHD split 3 ways yields $[3.334, 3.333, 3.333]$ BHD, perfectly summing to $10.000$ BHD.

### 2.2 Event Sourcing State Machine
Instead of maintaining a mutable balance row, account balances are dynamically derived from the immutable event stream:
$$\text{ClosingLedgerBalance}(A, d) = \sum \{ e.\text{Amount} \mid e.\text{Account} = A \land e.\text{ValueDate} \le d \land e \text{ is booked} \}$$

$$\text{AvailableBalance}(A) = \text{TotalLedgerBalance}(A) - \sum \{ h.\text{Amount} \mid h.\text{Account} = A \land h.\text{Status} = \text{APPROVED} \}$$

### 2.3 Concurrency & Thread-Safety Model
- The core ledger engine employs a read-write mutex (`sync.RWMutex`).
- **Read Operations:** (`CalculateClosingLedgerBalance`, `GetAvailableBalance`, `GetActiveHolds`) acquire shared read locks (`RLock`), allowing high-throughput concurrent auditing.
- **Write Operations:** (`PostCredit`, `PostDebit`, `RequestAuthorization`, `SettleAuthorization`, `ReverseTransaction`, `AssessOverdraftFees`) acquire exclusive write locks (`Lock`), ensuring deterministic sequential ordering (`l.eventSeq++`).
- Validated with Go's race detector (`go test -race ./...`).

---

## 3. Relational Persistence Architecture (MySQL 8.0)

When scaling beyond a single-node in-memory core to a distributed, persistent deployment, MySQL 8.0 (InnoDB) serves as a robust relational event store.

```
┌────────────────────────────────────────────────────────────────────────┐
│                        TRANSACTION BOUNDARY                            │
│                                                                        │
│  1. SELECT * FROM accounts WHERE account_id = ? FOR UPDATE             │
│  2. Validate Available Balance (Ledger Balance - Active Holds)         │
│  3. INSERT INTO ledger_entries (...) VALUES (...)                      │
│  4. INSERT/UPDATE authorization_holds (...) VALUES (...)               │
│  5. INSERT INTO outbox_events (...) VALUES (...)                       │
│  6. UPDATE accounts SET version = version + 1 WHERE account_id = ?     │
│                                                                        │
└───────────────────────────────────┬────────────────────────────────────┘
                                    │ Commit
                                    ▼
                     Binlog (Row-Based Replication)
                                    │
                                    ▼
                Debezium / Kafka CDC Engine (Outbox Stream)
```

### 3.1 Table Schema Breakdown (see `db/schema.sql`)
1. **`accounts`**:
   - Stores account metadata and optimistic locking `version`.
2. **`ledger_entries`**:
   - Append-only event store.
   - Compound index on `(account_id, value_day)` enables sub-millisecond range scans for value-date balance recalculations.
   - Unique index on `idempotency_key` guarantees exactly-once processing across network retries.
3. **`authorization_holds`**:
   - Fast state lookup for active holds (`idx_active_holds` on `(account_id, status)`).
4. **`daily_closing_snapshots`**:
   - Materialized view / read-model caching daily closing balances and fee counts.
5. **`outbox_events`**:
   - Transactional outbox table ensuring atomicity between ledger state and downstream message brokers (Kafka/RabbitMQ) without distributed 2PC.

### 3.2 Locking & Concurrency Control Trade-offs

| Strategy | Mechanism | Pros | Cons | Verdict for Financial Ledgers |
|---|---|---|---|---|
| **Pessimistic Locking** | `SELECT ... FOR UPDATE` on `accounts` row | Prevents race conditions at read time; guarantees serialized execution per account. | Lock contention under high hot-account throughput. | **ADOPTED for Authorizations & Debits:** Guarantees available funds are never double-spent. |
| **Optimistic Concurrency (OCC)** | `UPDATE accounts SET version = version + 1 WHERE account_id = ? AND version = ?` | Non-blocking reads; zero lock overhead for disjoint operations. | High retry rate under high concurrency. | **ADOPTED for Read-Heavy Account Queries & Snapshots.** |
| **Deadlock Avoidance** | Ordered Lock Acquisition | Eliminates cyclic wait conditions in multi-account operations. | Requires strict lexicographical ordering of account IDs before locking. | **MANDATORY for Double-Entry Transfers:** Always lock $\min(A, B)$ before $\max(A, B)$. |

### 3.3 Transaction Isolation Level
- **Recommended: `READ COMMITTED` with Explicit Row Locking (`FOR UPDATE`).**
  - Why not `REPEATABLE READ` (MySQL default)?
    Under MySQL InnoDB, `REPEATABLE READ` uses next-key locking (gap locks) which frequently leads to spurious deadlocks when inserting into indexed ranges.
  - In `READ COMMITTED`, gap locking is disabled (except for foreign key checks), allowing maximum insert throughput into the `ledger_entries` append-only table while row-level locks on `accounts` ensure atomic balance updates.

---

## 4. Trade-off Matrix: In-Memory vs MySQL vs Distributed EventStore

| Dimension | In-Memory Core (This Engine) | MySQL 8.0 Relational Store | Distributed Event Store (Kafka / EventStoreDB) |
|---|---|---|---|
| **Latency** | Sub-microsecond ($< 1\,\mu\text{s}$) | Low ($1 \text{ ms} - 5 \text{ ms}$) | Medium ($5 \text{ ms} - 20 \text{ ms}$) |
| **Throughput** | $> 1{,}000{,}000$ tx/sec | $5{,}000 - 20{,}000$ tx/sec | $> 100{,}000$ tx/sec |
| **Persistence** | Volatile (Process Memory) | ACID Durability (WAL / fsync) | Distributed Log Replication |
| **Backdated Event Cost** | Immediate in-memory rescan | Indexed B-Tree range query | Log replay / Projection rebuild |
| **Operational Complexity** | Zero dependencies | Standard DBA tooling | High cluster management overhead |

---

## 5. Live Defense Preparation: Architectural Defense Q&A

### Q1: Why did you refuse Acceptance Criterion 2 ("E7 causes exactly one overdraft fee on Day 2")?
**Defense:** "When E7 is backdated to Day 2, Day 2's closing balance becomes -AED 370.00, triggering an overdraft fee of AED 25.00 booked with `value_date = Day 2`. That fee further depresses the balance to -AED 395.00. This deduction cascades forward through Day 3 (+AED 5.00) into Day 4. On Day 4, settlements E5 (AED 185.00) and E6 (AED 180.00) drop Day 4's closing balance to -AED 360.00. Because Day 4 closing balance is negative, the non-negotiable rule mandates an overdraft fee for Day 4 as well. Therefore, E7 causes multiple overdraft fees across different days, not exactly one."

### Q2: Why represent money as `int64` minor units instead of `math/big.Rat` or `float64`?
**Defense:** "Float64 is unacceptable because IEEE 754 binary floating-point suffers from representation errors (e.g., $0.1 + 0.2 \ne 0.3$). `math/big.Rat` is exact for rational numbers, but it requires heap allocation on every arithmetic operation, degrading GC throughput. `int64` minor units (fils) provide exact integer precision with zero allocations, hardware-native SIMD speed, and guaranteed conservation of money."

### Q3: How do you handle un-matched settlements (E6 Auth-Z)?
**Defense:** "In closed-loop software, an un-matched authorization is an error. But under international card scheme rules (Visa / Mastercard), presentment clearing files contain Force Posts (offline captures). Issuing banks are contractually required to settle presentments even if no prior authorization exists or if it forces an overdraft. We designed the engine with a configurable policy: it supports strict refusal for closed-loop ledgers, and standard Force Post debiting for open-loop payment networks."

### Q4: Why can't E9 reversal restore fees or approve Auth-B?
**Defense:** "First, the ledger is append-only. Assessed fees represent historical, contractual penalties incurred while the account was legitimately overdrawn; they cannot vanish without an explicit waiver event. Second, authorization is a real-time point-of-sale decision. When Auth-B was presented on Day 5, the customer had negative funds and was declined. A reversal on Day 6 cannot travel backwards in time to approve a purchase that already failed at the cash register."
