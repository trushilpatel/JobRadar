#!/usr/bin/env bash
# Per-package coverage gate (statement-weighted).
#
# Usage: ./scripts/check-coverage.sh <coverage.out> <threshold-percent>
# Exits non-zero if any non-excluded package falls below the threshold.
#
# Statement-weighted: covered_statements / total_statements per package.
# Empty function bodies (which have zero statements) do not drag the
# average. This matches how Go itself reports the overall "total:" figure.

set -euo pipefail

COVER_FILE="${1:-/tmp/coverage.out}"
THRESHOLD="${2:-80}"

if [ ! -f "$COVER_FILE" ]; then
  echo "coverage file not found: $COVER_FILE" >&2
  exit 2
fi

# Packages / files excluded from the threshold:
#   - main entry points, generated wire output, transitional compat shims
#   - dto / models (data carriers — exercised through services)
#   - testutil (test helpers)
#   - common/db, common/jetstream — broker / Postgres-coupled infrastructure;
#     verified by the integration tier (.github/workflows/integration.yml)
EXCLUDE_REGEX='(/main|/wire_gen|/compat|/testutil|/dto|/models|/common/db|/common/jetstream)(/|$)'

# Files excluded individually: wire.go, generated wire_gen.go, any
# wire_*.go wiring helper, and every main.go entry point.
FILE_EXCLUDE_REGEX='/(wire(_[a-z_]+)?|wire_gen|main)\.go$'

# Profile lines look like:
#   github.com/x/y/file.go:start.col,end.col numStatements numHits
# With -coverpkg=./..., the profile contains DUPLICATE entries — one per
# test package that ran. Dedupe by (file:range), keeping MAX(hits), then
# aggregate per package (statement-weighted).
awk -v threshold="$THRESHOLD" -v exclude="$EXCLUDE_REGEX" -v fileExclude="$FILE_EXCLUDE_REGEX" '
  NR == 1 && $1 == "mode:" { next }
  NF < 3 { next }
  {
    key   = $1
    stmts = $2 + 0
    hits  = $3 + 0
    if (!(key in seen) || hits > seen[key]) {
      seen[key]  = hits
      sized[key] = stmts
    }
  }
  END {
    for (key in seen) {
      idx = index(key, ":")
      if (idx == 0) continue
      file = substr(key, 1, idx - 1)
      if (file ~ fileExclude) continue
      # Strip trailing /<basename> to get the package path.
      n = split(file, parts, "/")
      pkg = ""
      for (i = 1; i < n; i++) {
        if (i > 1) pkg = pkg "/"
        pkg = pkg parts[i]
      }
      if (pkg ~ exclude) continue
      stmts = sized[key]
      hits  = seen[key]
      total[pkg] += stmts
      if (hits > 0) covered[pkg] += stmts
      overall_total += stmts
      if (hits > 0) overall_covered += stmts
    }

    printf "%-8s %-8s %s\n", "COVER", "STATUS", "PACKAGE"
    printf "%-8s %-8s %s\n", "-----", "------", "-------"

    # Insertion sort package names for deterministic output.
    nk = 0
    for (p in total) keys[nk++] = p
    for (i = 1; i < nk; i++) {
      for (j = i; j > 0 && keys[j-1] > keys[j]; j--) {
        t = keys[j-1]; keys[j-1] = keys[j]; keys[j] = t
      }
    }

    fail = 0
    for (i = 0; i < nk; i++) {
      p = keys[i]
      if (total[p] == 0) continue
      pct = (covered[p] / total[p]) * 100.0
      status = (pct < threshold + 0) ? "FAIL" : "ok"
      if (status == "FAIL") fail = 1
      printf "%-7.1f%% %-8s %s\n", pct, status, p
    }

    print ""
    if (overall_total > 0) {
      overall = (overall_covered / overall_total) * 100.0
      printf "Overall coverage: %.1f%% (threshold: %s%% per package)\n", overall, threshold
    }

    if (fail) {
      print "" > "/dev/stderr"
      print "FAIL: one or more packages below threshold." > "/dev/stderr"
      exit 1
    }
    print "All packages meet the threshold."
  }' "$COVER_FILE"
