#!/usr/bin/env bash
set -euo pipefail

# Each invocation starts fresh Testcontainers-backed PostgreSQL state.
# Keep -count=1 so the Go test cache cannot conceal flaky concurrency tests.
repeats="${CONCURRENCY_REPEATS:-5}"
if ! [[ "$repeats" =~ ^[1-9][0-9]*$ ]]; then
  echo "CONCURRENCY_REPEATS must be a positive integer" >&2
  exit 2
fi

focus='should (retry concurrent transactions|serialize concurrent requests|serialize concurrent multi-entry requests|approve only one of two concurrent debits|assign unique monotonic sequences for concurrent transfers)'

for ((iteration=1; iteration<=repeats; iteration++)); do
  echo ">>> PostgreSQL concurrency suite: iteration ${iteration}/${repeats}"
  go test -race -tags=integration -count=1 -timeout=10m \
    ./tests/integration/postgres \
    -ginkgo.focus="$focus" \
    -ginkgo.fail-on-empty
done

echo "All ${repeats} concurrency test iterations passed."
