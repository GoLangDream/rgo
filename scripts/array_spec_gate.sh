#!/bin/bash
set -euo pipefail

ROOT=$(cd "$(dirname "$0")/.." && pwd)
OUT="$ROOT/reports/spec-status/array.csv"

(cd "$ROOT" && scripts/safe_go_test.sh ./...)
(cd "$ROOT" && scripts/feature_test.sh)
(cd "$ROOT" && RGO_SPEC_TIMEOUT="${RGO_SPEC_TIMEOUT:-5}" scripts/spec_status.sh vendor/ruby/spec/core/array "$OUT")

non_pass=$(awk -F, 'NR>1 && $2!="pass" {count++} END {print count+0}' "$OUT")
if [ "$non_pass" -ne 0 ]; then
	awk -F, 'NR>1 && $2!="pass" {print $2 " " $1}' "$OUT" >&2
	exit 1
fi
