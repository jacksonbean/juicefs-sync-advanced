#!/bin/bash
set -euo pipefail

BINARY="./juicefs-sync-advanced"
DB="/tmp/migration-report.db"
: "${NO_HTTPS:=}"

SRC="${1:?Usage: $0 <src> <dst>}"
DST="${2:?Usage: $0 <src> <dst>}"
LIMIT="${LIMIT:--1}"
REPORT="${REPORT:-migration-report.csv}"

echo "============================================"
echo " Migration Performance Test"
echo " Source: $SRC"
echo " Dest:   $DST"
echo "============================================"
echo ""

cleanup() { rm -f "$DB"; }
trap cleanup EXIT

# Phase 1: Scan source
echo "[1/4] Scanning source..."
START=$(date +%s)
$BINARY scan $NO_HTTPS --db-type sqlite3 --db-dsn "$DB" --scan-id src --limit "$LIMIT" "$SRC" 2>&1
SCAN_SRC_TIME=$(( $(date +%s) - START ))

# Phase 2: dry-run sync
echo ""
echo "[2/4] Dry-run sync..."
START=$(date +%s)
$BINARY sync --dry $NO_HTTPS --record-db-type sqlite3 --record-db-dsn "$DB" --record-run-id dryrun "$SRC" "$DST" 2>&1
DRY_TIME=$(( $(date +%s) - START ))

# Phase 3: real sync
echo ""
echo "[3/4] Running actual sync..."
START=$(date +%s)
$BINARY sync $NO_HTTPS --record-db-type sqlite3 --record-db-dsn "$DB" --record-run-id real "$SRC" "$DST" 2>&1
SYNC_TIME=$(( $(date +%s) - START ))

# Phase 4: scan destination + diff
echo ""
echo "[4/4] Verifying..."
START=$(date +%s)
$BINARY scan $NO_HTTPS --db-type sqlite3 --db-dsn "$DB" --scan-id dst --limit "$LIMIT" "$DST" 2>&1
$BINARY scan --db-type sqlite3 --db-dsn "$DB" --diff-from src --diff-to dst --export "$REPORT" 2>&1
VERIFY_TIME=$(( $(date +%s) - START ))

# Summary
echo ""
echo "============================================"
echo " Migration Report"
echo "============================================"
echo ""

SCANNED=$(sqlite3 "$DB" "SELECT total_scanned FROM sync_runs WHERE run_id='real'" 2>/dev/null || echo "N/A")
COPIED=$(sqlite3 "$DB" "SELECT copied FROM sync_runs WHERE run_id='real'" 2>/dev/null || echo "N/A")
COPIED_BYTES=$(sqlite3 "$DB" "SELECT copied_bytes FROM sync_runs WHERE run_id='real'" 2>/dev/null || echo "N/A")
DIFF_COUNT=$(tail -n +2 "$REPORT" 2>/dev/null | wc -l | tr -d ' ' || echo "0")

echo " Source scan:        ${SCAN_SRC_TIME}s"
echo " Dry-run scan:       ${DRY_TIME}s"
echo " Sync time:          ${SYNC_TIME}s"
echo " Verification:       ${VERIFY_TIME}s"
echo "--------------------"
echo " Objects scanned:    $SCANNED"
echo " Objects copied:     $COPIED"
echo " Bytes copied:       $COPIED_BYTES"
echo " Post-sync diffs:    $DIFF_COUNT"
echo "--------------------"
if [ "$DIFF_COUNT" = "0" ]; then
    echo " Status:             ✓ PASS (no differences)"
else
    echo " Status:             ✗ FAIL ($DIFF_COUNT differences found)"
fi
echo ""
echo "Detailed diff: $REPORT"
