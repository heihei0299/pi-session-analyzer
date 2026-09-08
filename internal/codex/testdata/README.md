# Codex synthetic fixtures

These files contain only synthetic IDs, paths, models, and token counts.

- `codex-home/sessions`: metadata, turn model, duplicate/conflict response, missing reliable usage, cumulative snapshot, reroute, unknown event, malformed JSON, and reverted rollout identity.
- `codex-home/archived_sessions`: archived rollout discovery and usage mapping.
- zstd and half-line cases are generated in Go tests so no binary/compressed fixture is committed.
