#!/bin/bash
set -e
cd "$(dirname "$0")"
echo "=== Init git ===" && git init
echo "=== Stage files ===" && git add -A
echo "=== Commit ===" && git commit -m "juicefs-sync-advanced: UI migration, REST API, Web dashboard, bug fixes"
echo "=== Add remote ===" && git remote add origin https://github.com/jacksonbean/juicefs-sync-advanced.git 2>/dev/null || git remote set-url origin https://github.com/jacksonbean/juicefs-sync-advanced.git
echo "=== Push ===" && git push -u origin main
echo "=== Done ==="
