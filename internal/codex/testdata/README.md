# Codex synthetic fixtures

These files contain only synthetic IDs, paths, models, and token counts. Thread/rollout ids are
placeholders of the form `00000000-0000-7000-8000-00000000000N` — never copy an id from a real Codex
home into a fixture.

The layout mirrors a real Codex home: canonical rollout names are
`rollout-<second-granularity timestamp>-<thread id>[_<rollout id>].jsonl`, nested under
`<year>/<month>/<day>` directories. The timestamp is upstream local wall-clock time, and discovery
only validates its shape — it is never interpreted as an instant.

- `codex-home/sessions/2026/09/08`: metadata, turn model, duplicate/conflict response, missing reliable usage, cumulative snapshot, reroute, unknown event, malformed JSON, and a reverted rollout (same thread id, separate rollout id). Every usage record declares the upstream `usage.total_tokens`, so the fixture doubles as the source of truth for the token semantics.
- `codex-home/sessions/2026/09/08/rollout-…-000000000004.jsonl`: snapshot-only rollout — two `token_count` events and a `world_state` event, no durable usage record. It must contribute no usage row, must be reported as uncounted coverage, and must not be reported as an unknown event type.
- `codex-home/sessions/2026/09/08/notes.jsonl`: non-canonical artifact that must be skipped with a diagnostic instead of parsed.
- `codex-home/archived_sessions/2026/09/07`: archived rollout discovery and usage mapping.
- zstd and half-line cases are generated in Go tests so no binary/compressed fixture is committed.
