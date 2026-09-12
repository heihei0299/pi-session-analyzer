# Final Sign-off Audit — 2026-09-12

## Scope

- Repository: `heihei0299/pi-session-analyzer`
- Fixed point: `8df1bbad4ccfd7ec7d875e5564a2fd417163c627`
- Reviewed head: `d95d945cf6ce912a6eaaf444024baf8db86c2ffa`
- Reviewed change: `fix: close storage and standalone migration contracts`
- Method: static code review only in this audit pass. No local build/test/lint/typecheck was executed by the reviewer.

## Executive summary

The Go-only migration is functionally near completion. The previous transaction, cursor, discovery, watch retry, rename consistency, Codex scope and standalone-CI issues are materially resolved. The remaining blocker set is now small and concentrated.

Current sign-off status: **NOT READY FOR FINAL SIGN-OFF**.

Remaining findings:

- **P0: 1** — destructive rollup/prune scope in shared databases.
- **P1: 1** — standalone OpenCode semantic identity still differs from token-analyzer canonical semantics for usage payloads.

No new architecture rewrite is required. Both findings should be fixed inside tickets 06/07.

---

## Standards findings

### A-01 — P0 — RollupAndPrune can prune unrelated shared-database rows

**Files**

- `internal/db/maintenance.go`
- `internal/refresh/refresh.go`
- `CONTEXT.md`

**Observed behavior**

`RollupAndPrune` currently aggregates and deletes all rows older than the retention cutoff:

```sql
FROM proxy_request_logs
WHERE created_at < ?
```

and later:

```sql
DELETE FROM proxy_request_logs
WHERE created_at < ?
```

There is no ownership predicate restricting maintenance to rows owned by token-analyzer sources.

At the same time, the project explicitly supports using a shared cc-switch database through `TOKEN_ANALYZER_DB` / `--db`. In that configuration, `proxy_request_logs` may contain rows written by other applications or providers.

**Impact**

A normal token-analyzer refresh can roll up and delete unrelated historical rows from a shared database. This is a destructive cross-application mutation and is therefore release-blocking.

The problem is amplified by the new source-specific watcher because every successful `Refresh(pi)` or `Refresh(codex)` still calls global `RollupAndPrune`.

**Required correction**

Define one shared maintenance ownership predicate and use it identically for both rollup SELECT and raw DELETE. At minimum, maintenance must only touch token-analyzer-owned rows, for example:

```sql
(
  (app_type = 'pi' AND data_source = 'pi_session')
  OR
  (app_type = 'codex' AND data_source = 'codex')
)
```

The exact predicate should follow the accepted storage contract, but INSERT and DELETE must never drift.

**Required regression**

Create a shared-database fixture containing:

- old Pi row;
- old Codex row;
- old generic proxy row;
- old unrelated app row;
- recent rows for each class.

After maintenance:

- eligible token-analyzer rows may be rolled/pruned;
- generic proxy/unrelated rows must remain byte-for-byte unaffected at the logical-row level;
- repeated maintenance must stay idempotent.

**Severity rationale**

P0 because this can delete data not owned by token-analyzer.

---

### A-02 — P1 — standalone OpenCode usage identity is not canonical-equivalent

**Files**

- `opencode-analyzer/internal/piaudit/records.go`
- `internal/pi/identity.go`
- `opencode-analyzer/internal/piaudit/audit_contract_test.go`

**Observed behavior**

The main token-analyzer semantic identity hashes the complete `usage` object after selecting canonical message fields.

The standalone OpenCode analyzer instead calls `canonicalUsage(usage)` and keeps only a subset:

- input
- output
- cacheRead
- cacheWrite
- reasoning
- totalTokens
- cost.total

This means two no-entry-id records that differ only in another usage field can be distinct in token-analyzer but collapse to the same standalone semantic ID.

The new standalone regression covers irrelevant **message** metadata, but not extra **usage** fields.

**Impact**

Standalone audit can deduplicate requests differently from token-analyzer, violating ticket 07's semantic-contract requirement.

**Required correction**

Keep standalone implementation independent, but preserve the same equivalence relation as token-analyzer:

- canonical message field selection stays explicit;
- the complete usage object participates in semantic identity;
- JSON object key order must not change identity.

Byte-for-byte hash equality between modules is not required; semantic equivalence is.

**Required regression matrix**

1. same canonical fields + different irrelevant message metadata => dedup;
2. different canonical message field => do not dedup;
3. same billing totals + different extra usage field => do not dedup;
4. same usage object with different map/key order => dedup.

---

## Spec review

### Ticket 06 — storage/runtime hardening

**Implemented successfully**

- real transactional rollup + delete;
- rollback on rollup failure;
- reasoning preserved in rollup schema;
- partial-day boundary rollups excluded from exact aggregation;
- `coverageStatus=partial` and warning exposed when boundary-day rollups exist;
- Pi `sync_semantics_version` introduced;
- old cursor semantics trigger rescan;
- Pi/Codex source-specific watcher behavior;
- Codex cheap physical fingerprint avoids full decode on every poll;
- watch acknowledgment advances only after successful refresh.

**Not accepted yet**

Ticket 06 cannot be considered complete while maintenance can mutate unrelated shared-database rows (A-01).

Recommended tracker state until fixed: `in-progress`.

### Ticket 07 — standalone OpenCode / CI / docs

**Implemented successfully**

- standalone Go module is independently tested in CI;
- irrelevant message metadata regression exists;
- tracker checklists 03/04/05 were reconciled;
- AGENTS/README/CONTEXT/ADR cleanup was applied;
- standalone module remains structurally independent.

**Not accepted yet**

Ticket 07's canonical semantic identity requirement is still partial because usage-object equivalence differs (A-02).

Recommended tracker state until fixed: `in-progress`.

---

## Previously reported issues now considered resolved by static review

- Pi sync uses a real `sql.Tx`.
- Usage/dedup/session/cursor mutations are transactional.
- Mutation errors are propagated.
- Cross-refresh partial → final replacement exists.
- Production recursive Pi discovery fallback is removed.
- Codex sibling-prefix path scoping is path-aware.
- Sorting is deterministic with explicit tie-breakers.
- Watch failures remain retryable.
- Same-size rewrite detection exists for Pi/Codex.
- Rename refreshes the ledger snapshot before success.
- Legacy SessionData usage parser/cache production path is removed.
- `make build` outputs `dist/token-analyzer`.
- standalone OpenCode module has a dedicated CI test step.

---

## Final sign-off gates

The migration may be signed off only when all of the following are true:

1. Rollup/prune maintenance is restricted to token-analyzer-owned rows.
2. Shared-database preservation regression proves unrelated rows are untouched.
3. Rollup SELECT and DELETE use the same ownership scope.
4. Standalone OpenCode semantic identity uses the same usage-object equivalence as token-analyzer.
5. Standalone identity regressions cover message metadata and usage metadata cases.
6. Ticket 06 and 07 answers/checklists reflect the corrected implementation.
7. Authorized verification passes for root module and standalone module.

## Recommended verification after fixes

Only run when explicitly authorized:

```bash
env -u TOKEN_ANALYZER_DB GOMAXPROCS=2 go test -p 1 ./...
(cd opencode-analyzer && GOMAXPROCS=2 go test -p 1 ./...)
```

Optional release verification when authorized:

```bash
make build
make release
```

## Sign-off assessment

Estimated completion: **90–93%**.

The architecture is complete. Remaining work is narrowly scoped correctness hardening, not redesign.
