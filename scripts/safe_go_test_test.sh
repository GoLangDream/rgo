#!/bin/bash
set -euo pipefail

ROOT=$(cd "$(dirname "$0")/.." && pwd)
WORK=$(mktemp -d /tmp/rgo_safe_go_test_XXXXXX)
trap 'rm -rf "$WORK"' EXIT

mkdir -p "$WORK/example"

cat > "$WORK/example/go.mod" <<'GO'
module example

go 1.22
GO

cat > "$WORK/example/example_test.go" <<'GO'
package example

import "testing"

func TestPass(t *testing.T) {}

func TestSkipTarget(t *testing.T) {
    t.Fatal("this test should not run")
}
GO

cd "$WORK/example"

out="$WORK/out"
RGO_GO_TEST_TIMEOUT=5 "$ROOT/scripts/safe_go_test.sh" -run '^TestPass$' . >"$out" 2>&1
grep -q 'ok[[:space:]]\+example' "$out"

echo "safe_go_test_test: PASS"
