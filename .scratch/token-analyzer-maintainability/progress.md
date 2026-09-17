# token-analyzer-maintainability Progress

## Preflight

- `HEAD`: `df8df63f282938bab624b30c9e69918d6b4d807e`
- `BASE_HEAD`: `df8df63f282938bab624b30c9e69918d6b4d807e`
- Branch: `main`
- Existing workspace changes: OpenCode Analyzer migration-out cleanup in tracked root files and deletion of `opencode-analyzer/`; these are pre-existing and out of scope for this feature. Do not overwrite, revert, or mix them into issue commits.
- Confirmed module: `github.com/heihei0299/token-analyzer` (Go 1.23, standard library plus existing SQLite/compression dependencies).
- Planned targeted test command: `GOMAXPROCS=2 go test -p 1 <affected packages/tests>`.
- Planned static check: `gofmt -l <changed Go files>` and `go vet <affected packages>`.
- Planned full convergence command (A4 only): `GOMAXPROCS=2 go test -p 1 ./...`.
- Planned build/release checks when required and authorized: `go build ./cmd/token-analyzer`, `make release`, and `--help`/`--version` smoke checks.
- Real runtime verification: existing Go `httptest`, CLI command seam, canonical fixture → Refresh → ledger → Query tests; no browser harness is required by the spec.

## DAG

- `01 → 03, 04, 05, 07`
- `02 → 04, 05, 07`
- `03 → 06, 07`
- `04 → 06, 07`
- `05 → 06, 07`
- `06 → 07`

## Layers (Kahn L1..Ln)

- L1: `01, 02`
- L2: `03, 04, 05`
- L3: `06`
- L4: `07`

## Progress

| NN | Status | Commit | Review | Tests |
|---|---|---|---|---|
| 01 | done | pending aggregate commit | pending | `GOMAXPROCS=2 go test -p 1 ./internal/query`; `GOMAXPROCS=2 go vet ./internal/query` (pass) |
| 02 | done | pending aggregate commit | pending | `GOMAXPROCS=2 go test -p 1 ./internal/db ./internal/query`; `go vet ./internal/db ./internal/query` (pass) |
| 03 | done | pending aggregate commit | pending | `TOKEN_ANALYZER_DB= GOMAXPROCS=2 go test -p 1 ./cmd/token-analyzer ./internal/server ./internal/refresh ./internal/query ./internal/timerange`; `go vet` (pass) |
| 04 | done | pending aggregate commit | pending | `TOKEN_ANALYZER_DB= GOMAXPROCS=2 go test -p 1 ./internal/pi ./internal/codex ./internal/query ./internal/server ./cmd/token-analyzer ./internal/refresh`; `go vet` (pass) |
| 05 | done | pending aggregate commit | pending | `TOKEN_ANALYZER_DB= GOMAXPROCS=2 go test -p 1 ./internal/query ./internal/server ./cmd/token-analyzer ./internal/pi ./internal/codex ./internal/db`; `go vet` (pass) |
| 06 | done | — | pending | `TOKEN_ANALYZER_DB= GOMAXPROCS=2 go test -p 1 ./internal/query ./internal/server ./cmd/token-analyzer ./internal/pi ./internal/codex ./internal/db`; `TOKEN_ANALYZER_DB= GOMAXPROCS=2 go vet ./internal/query ./internal/server ./cmd/token-analyzer ./internal/pi ./internal/codex ./internal/db` (pass) |
| 07 | done | — | pending | `TOKEN_ANALYZER_DB= GOMAXPROCS=2 go test -p 1 ./internal/server ./cmd/token-analyzer`; `TOKEN_ANALYZER_DB= GOMAXPROCS=2 go vet ./internal/server ./cmd/token-analyzer`; `gofmt -l` changed Go files and `make -n` version dry-runs (pass) |
