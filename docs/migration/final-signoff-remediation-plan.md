# Final Sign-off Remediation Plan

## Goal

Close the last two correctness blockers found at `d95d945cf6ce912a6eaaf444024baf8db86c2ffa` without changing the accepted Go-only architecture.

This is a narrow remediation pass. Do not introduce new sources, new runtime layers, new storage abstractions, or re-couple OpenCode into token-analyzer.

## Execution order

```text
F1 Shared-DB-safe maintenance
        ↓
F2 Standalone semantic identity parity
        ↓
F3 Tracker/docs closeout
        ↓
F4 Authorized verification + final sign-off
```

---

## F1 — Make rollup/prune safe in shared databases

### Objective

Ensure token-analyzer maintenance mutates only rows it owns, even when `TOKEN_ANALYZER_DB` or `--db` points at a shared cc-switch-compatible database.

### Primary files

- `internal/db/maintenance.go`
- `internal/db/maintenance_test.go`
- `internal/refresh/refresh.go` only if orchestration needs a small adjustment
- `CONTEXT.md`
- `.scratch/go-only-backend-migration/issues/06-storage-runtime-hardening.md`

### Required implementation

1. Define a single ownership predicate for maintenance-eligible usage rows.

   Recommended accepted scope:

   ```text
   Pi    => app_type='pi'    AND data_source='pi_session'
   Codex => app_type='codex' AND data_source='codex'
   ```

   If current adapter constants differ, use the actual canonical values from the source adapters.

2. Use the exact same predicate in:

   - rollup INSERT/SELECT source rows;
   - raw DELETE.

3. Keep cutoff behavior unchanged:

   ```text
   created_at < now - 30d
   ```

4. Preserve the existing transaction boundary:

   ```text
   BEGIN
     aggregate owned expired rows
     delete exactly those owned expired rows
   COMMIT
   incremental_vacuum (best effort after commit)
   ```

5. Do not make maintenance source-dependent in a way that can strand data indefinitely. Calling maintenance after either Pi or Codex refresh is acceptable as long as it only touches the explicitly owned row classes.

### Required tests

Add a shared-database preservation test with at least:

```text
old Pi owned row
old Codex owned row
old generic proxy row
old unrelated app row
recent Pi owned row
recent Codex owned row
recent generic proxy row
```

Assert after maintenance:

- old Pi/Codex rows are represented in rollups and removed from raw;
- recent Pi/Codex rows remain raw;
- generic proxy/unrelated rows remain raw regardless of age;
- no rollup row is created from generic proxy/unrelated rows;
- second maintenance run changes nothing logically;
- injected rollup failure leaves both raw and rollup tables unchanged.

### Definition of done

- No SQL maintenance statement has a broad `created_at < ?` mutation without the ownership predicate.
- Shared DB cannot lose rows written by other applications.
- Ticket 06 can truthfully claim rollup/prune lifecycle completion.

### Recommended commit

```text
fix(storage): scope rollup maintenance to owned ledger rows
```

---

## F2 — Make standalone OpenCode semantic identity equivalent

### Objective

Keep `opencode-analyzer` implementation independent while making its no-entry-id semantic dedup equivalence match token-analyzer's canonical Pi identity.

### Primary files

- `opencode-analyzer/internal/piaudit/records.go`
- `opencode-analyzer/internal/piaudit/audit_contract_test.go`
- optionally a dedicated `identity_test.go`
- `.scratch/go-only-backend-migration/issues/07-opencode-ci-doc-closeout.md`

### Required implementation

1. Keep the existing explicit canonical message field selection:

   ```text
   provider
   model
   responseModel
   responseId
   api
   toolCallId
   toolName
   stopReason
   errorMessage
   content
   message timestamp
   entry timestamp
   kind
   summary when applicable
   ```

2. Replace filtered `canonicalUsage(usage)` semantics with the complete usage object for semantic identity.

3. Ensure JSON/map key order does not change semantic identity. Go's standard `encoding/json` map-key ordering is deterministic for string-keyed maps, so the standalone implementation may rely on deterministic serialization if tests prove the behavior.

4. Do not import token-analyzer `internal/pi`; standalone remains self-contained.

5. Byte-for-byte hash equality across modules is not required. The required contract is equivalence:

   ```text
   token-analyzer considers A/B the same semantic request
   iff
   opencode-analyzer considers A/B the same semantic request
   ```

### Required tests

Add focused identity regressions:

#### Case 1 — irrelevant message metadata

Same canonical message fields and same usage, but different unrelated message metadata.

Expected: **dedup**.

#### Case 2 — canonical message field changes

Example: model or responseId changes.

Expected: **do not dedup**.

#### Case 3 — extra usage field changes

Same known token totals, but the complete usage object differs in an additional field.

Expected: **do not dedup**, matching token-analyzer.

#### Case 4 — usage key order changes only

Same usage object serialized/constructed with different insertion order.

Expected: **dedup**.

#### Case 5 — request ID replacement remains intact

Same entry ID/timestamp; later finalized record wins according to existing stop/output rule.

Expected: one final record.

### Definition of done

- Standalone no-entry-id dedup uses canonical message selection + complete usage object.
- The four equivalence tests above pass.
- No dependency from `opencode-analyzer` to token-analyzer private/internal packages is introduced.
- Ticket 07 can truthfully claim semantic contract parity.

### Recommended commit

```text
fix(opencode): align standalone semantic usage identity
```

---

## F3 — Tracker and documentation closeout

### Objective

Make repository state describe what is actually verified after F1/F2.

### Files

- `.scratch/go-only-backend-migration/issues/06-storage-runtime-hardening.md`
- `.scratch/go-only-backend-migration/issues/07-opencode-ci-doc-closeout.md`
- `CONTEXT.md`
- `README.md` only if behavior wording changes
- `docs/audit/final-signoff-audit-2026-09-12.md` if the project records superseding status directly in historical audit docs

### Tasks

1. Keep 06/07 as `in-progress` while F1/F2 are unresolved.
2. Mark them `resolved` only after implementation and authorized verification satisfy acceptance.
3. In ticket Answers, record the exact fix commits.
4. Document maintenance ownership explicitly:

   ```text
   shared DB support does not grant token-analyzer ownership over unrelated proxy_request_logs rows.
   ```

5. Document standalone identity as:

   ```text
   canonical message fields + complete usage payload
   ```

6. Do not rewrite historical audit findings as if they never existed; add a resolved/superseded note instead.

### Recommended commit

```text
chore(contract): close final migration audit findings
```

---

## F4 — Verification and sign-off

### Authorization rule

Build/test/lint/typecheck must only be run with explicit user authorization.

### Minimum root verification

```bash
env -u TOKEN_ANALYZER_DB GOMAXPROCS=2 go test -p 1 ./...
```

Required regression coverage must include:

- shared DB preservation;
- rollup idempotency;
- rollback on maintenance failure;
- partial-day coverage;
- Pi sync semantics migration;
- source-specific watch retry;
- Codex cheap fingerprint.

### Standalone verification

```bash
cd opencode-analyzer
GOMAXPROCS=2 go test -p 1 ./...
```

Required regression coverage must include:

- four Pi carriers;
- billable/cost/failed gate;
- fork copied-history exclusion;
- request-ID final replacement;
- irrelevant message metadata dedup;
- canonical message field distinction;
- complete usage-object distinction;
- map/key order stability.

### Optional release verification

When explicitly authorized:

```bash
make build
make release
```

### Final sign-off checklist

```text
[ ] maintenance touches only token-analyzer-owned rows
[ ] shared DB unrelated rows survive maintenance unchanged
[ ] rollup SELECT and DELETE ownership predicates are identical
[ ] standalone semantic identity matches canonical equivalence
[ ] root tests pass
[ ] standalone tests pass
[ ] tickets 06/07 accurately marked resolved
[ ] docs describe final behavior
[ ] final code review has no P0/P1 findings
```

---

## Out of scope

Do not use this remediation to:

- restore TypeScript/Node backend;
- move `opencode-analyzer` to another repository now;
- redesign WebUI;
- change token accounting semantics;
- add new data sources;
- introduce a generic maintenance framework;
- alter retention duration unless a separate decision requests it.

## Expected end state

```text
Pi ──────┐
         ├── Refresh ── owned normalized ledger rows ── Query Engine
Codex ───┘          │
                    └── safe rollup/prune (owned rows only)

OpenCode Analyzer
  └── standalone Pi audit
      └── canonical message fields + complete usage identity
```

After F1–F4, the Go-only migration should be ready for final sign-off rather than another architecture iteration.
