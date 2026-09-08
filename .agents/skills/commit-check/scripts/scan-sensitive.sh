#!/usr/bin/env bash
# Deterministic staged-diff secret scan for commit-check.
# FAIL: structured assignments and private-key blocks.
# WARN: bare keywords that may legitimately appear in documentation.
set -euo pipefail

if [[ $# -gt 1 || ( $# -eq 1 && "$1" != "--staged-only" ) ]]; then
  echo "usage: $0 [--staged-only]" >&2
  exit 2
fi

fail_patterns='(api[_-]?key|secret|token|passwd|password)[[:space:]]*[=:][[:space:]]*[^[:space:]]{8,}|BEGIN (RSA|OPENSSH|EC|DSA) PRIVATE KEY'
warn_patterns='(api[_-]?key|secret|token|passwd|password|\.env)'

staged_diff=$(git diff --cached -U0)
fail=0

if grep -inE "$fail_patterns" <<< "$staged_diff"; then
  echo "❌ Structured secrets found in STAGED diff — remove them before committing." >&2
  fail=1
else
  echo "✅ No structured secrets in staged diff."
fi

if grep -inE "$warn_patterns" <<< "$staged_diff"; then
  echo "⚠  Keyword matches in STAGED diff — eyeball whether they are real secrets." >&2
fi

exit "$fail"
