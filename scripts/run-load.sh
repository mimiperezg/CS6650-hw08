#!/usr/bin/env bash
# Runs the shopping cart load test at 10, 50 and 100 users, 4 minutes each,
# with a 60 second pause between levels so each level shows up as its own
# step on CloudWatch's 1-minute graphs. Saves everything to
# results/<store>-load-<run>/:
#
#   config.txt        host, store, code version, and each level's start/end
#                     in UTC (use these as the CloudWatch time range)
#   u10/ u50/ u100/   per level: locust_stats.csv, locust_stats_history.csv,
#                     locust_failures.csv, report.html, locust.log
#
# Usage:
#   scripts/run-load.sh <run> [host]
#
# Without a host, it asks scripts/task-ip.sh for the ECS task's URL.
# An existing results/<store>-load-<run>/ is never overwritten.
#
# Optional settings:
#   STORE=dynamodb   name the results folder for the store under test
#                    (default mysql); it doesn't change the requests sent
#   DURATION=30s     length of each level (default 4m)
#   GAP=5            seconds between levels (default 60)

set -euo pipefail

if [[ $# -lt 1 || $# -gt 2 ]]; then
  sed -n '2,22p' "$0" | sed 's/^# \{0,1\}//'
  exit 2
fi

RUN="$1"
STORE="${STORE:-mysql}"
LEVELS="10 50 100"
SPAWN_RATE=10
DURATION="${DURATION:-4m}"
GAP="${GAP:-60}"

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
NAME="$STORE-load-$RUN"
OUT="$REPO_DIR/results/$NAME"
LOCUST="$REPO_DIR/locust/.venv/bin/locust"

if [[ -e "$OUT" ]]; then
  echo "results/$NAME already exists. Pick a new run name." >&2
  exit 1
fi

if [[ ! -x "$LOCUST" ]]; then
  echo "Locust isn't installed. Set it up first (see locust/README.md):" >&2
  echo "  python3 -m venv locust/.venv && locust/.venv/bin/pip install -r locust/requirements.txt" >&2
  exit 1
fi

if [[ $# -eq 2 ]]; then
  HOST="${2%/}"
else
  HOST="$("$REPO_DIR/scripts/task-ip.sh")"
fi

if ! curl -sf -o /dev/null --max-time 5 "$HOST/metrics"; then
  echo "The service isn't answering at $HOST/metrics." >&2
  exit 1
fi

utc() { date -u +%Y-%m-%dT%H:%M:%SZ; }

mkdir -p "$OUT"
{
  echo "run:        $RUN"
  echo "store:      $STORE"
  echo "host:       $HOST"
  echo "commit:     $(git -C "$REPO_DIR" rev-parse --short HEAD)$(git -C "$REPO_DIR" diff --quiet HEAD || echo ' (uncommitted changes)')"
  echo "locust:     $("$LOCUST" --version 2>&1 | head -1)"
  echo "user class: CartUser (locust/cartfile.py)"
  echo "levels:     $LEVELS users, spawn rate $SPAWN_RATE/s, $DURATION each, ${GAP}s apart"
  echo
} >"$OUT/config.txt"

FIRST=true
for USERS in $LEVELS; do
  if [[ "$FIRST" == true ]]; then
    FIRST=false
  else
    echo "Waiting ${GAP}s before the next level..."
    sleep "$GAP"
  fi

  DIR="$OUT/u$USERS"
  mkdir -p "$DIR"
  START="$(utc)"
  echo "=== $USERS users from $START ==="

  # Locust exits non-zero when any request failed. Record that and keep
  # going: failures under load are a result, not a reason to skip levels.
  set +e
  "$LOCUST" -f "$REPO_DIR/locust/cartfile.py" CartUser --host "$HOST" \
    --headless -u "$USERS" -r "$SPAWN_RATE" -t "$DURATION" \
    --csv "$DIR/locust" --html "$DIR/report.html" 2>&1 | tee "$DIR/locust.log"
  STATUS=${PIPESTATUS[0]}
  set -e

  END="$(utc)"
  echo "u$USERS: $START to $END (locust exit $STATUS)" >>"$OUT/config.txt"
done

echo
echo "Done. Results in results/$NAME/. CloudWatch time windows (UTC):"
sed -n '/^u[0-9]*: /p' "$OUT/config.txt"
