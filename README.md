# juicefs-sync-advanced

**A streamlined object storage sync tool** — extracted from [JuiceFS](https://github.com/juicedata/juicefs), focused solely on the `sync` command with enhanced dry-run, database recording, and performance analysis capabilities.

## Features

- **Multi-cloud sync** — sync objects between S3, OSS, COS, GCS, Azure Blob, local filesystem, and 30+ storage backends
- **Dry-run mode** (`--dry`) — scan and compare source/destination without copying, records planned actions to database
- **Database recording** — record scan results to MySQL, PostgreSQL, or SQLite for analysis
- **Plan table** — see exactly what would be copied, skipped, deleted, or updated before running
- **list-extra / list-lost** — audit mode to list orphaned or missing keys without modifying anything
- **Performance metrics** — Prometheus metrics + `sync_runs` summary table for migration performance benchmarking
- **Include/exclude filters** — rsync-compatible pattern matching
- **Cluster mode** — distributed sync with manager/worker architecture
- **Checksum verification** — optional end-to-end data integrity checking
- **Bandwidth limiting** — per-process and global traffic control

## Quick Start

### Build

```bash
go build -o juicefs-sync-advanced .
```

### Sync between two S3 buckets

```bash
./juicefs-sync-advanced sync s3://bucket1/path/ s3://bucket2/path/ -p 20
```

### Dry-run with SQLite recording

```bash
./juicefs-sync-advanced sync \
  --dry \
  --record-db-type sqlite3 \
  --record-db-dsn /tmp/sync_analysis.db \
  s3://source-bucket/ s3://dest-bucket/
```

### Analyze results

```sql
-- What would be copied?
SELECT action, count(*), sum(size) FROM sync_plan
WHERE run_id = 'dryrun-1700000000123456789'
GROUP BY action;

-- Performance summary
SELECT * FROM sync_runs ORDER BY started_at DESC;
```

## Usage

```
juicefs-sync-advanced sync [command options] SRC DST
```

### Key flags

| Flag | Description |
|------|-------------|
| `--dry` | Scan only, don't copy files |
| `-p, --threads` | Number of concurrent threads (default: 10) |
| `--update, -u` | Skip if destination is newer |
| `--force-update, -f` | Always update existing files |
| `--exclude PATTERN` | Exclude files matching pattern |
| `--include PATTERN` | Don't exclude files matching pattern (use with --exclude) |
| `--delete-src` | Delete source files that exist in destination |
| `--delete-dst` | Delete extra files on destination |
| `--check-all` | Verify integrity of all files |
| `--check-new` | Verify integrity of newly copied files |
| `--list-extra` | Print extra keys (in destination but not source) to stderr |
| `--list-lost` | Print lost keys (in source but not destination) to stderr |
| `--bwlimit` | Bandwidth limit in Mbps |
| `--start KEY` | First key to sync |
| `--end KEY` | Last key to sync |
| `--limit N` | Limit number of objects to process |

### Record / Database flags

| Flag | Description |
|------|-------------|
| `--record-db-type` | Database type: `mysql`, `postgres`, or `sqlite3` |
| `--record-db-dsn` | Database DSN (connection string) |
| `--record-table` | Table name for sync records (default: `objects`) |
| `--record-plan-table` | Table name for dry-run plan records (default: `sync_plan`) |
| `--record-run-id` | Custom run ID (auto-generated if not set) |
| `--record-extended-fields` | Enable extended fields (md5, retention, etc.) |

### URI format

```
[NAME://][ACCESS_KEY:SECRET_KEY[:TOKEN]@]BUCKET[.ENDPOINT][/PREFIX]
```

Supported storage types: `s3`, `oss`, `cos`, `obs`, `gs`, `wasb`, `minio`, `file`, `sftp`, `nfs`, `hdfs`, and many more.

### Examples

**Sync with include/exclude patterns:**
```bash
./juicefs-sync-advanced sync --include='logs/**' --exclude='temp/**' s3://src/ /mnt/dst/
```

**Find orphaned files in destination (no actual sync):**
```bash
./juicefs-sync-advanced sync --dry --list-extra s3://src/ s3://dst/
# Outputs: [Extra] <key>  for each file only in destination
```

**Find missing files in destination (no actual sync):**
```bash
./juicefs-sync-advanced sync --dry --list-lost s3://src/ s3://dst/
# Outputs: [Lost] <key>  for each file only in source
```

**Migration analysis with Postgres:**
```bash
./juicefs-sync-advanced sync --dry \
  --record-db-type postgres \
  --record-db-dsn "host=localhost user=app dbname=sync_analysis" \
  --record-plan-table migration_2025 \
  s3://old-bucket/ s3://new-bucket/
```

**Cluster mode (distributed sync):**
```bash
# Manager node
./juicefs-sync-advanced sync s3://src/ s3://dst/ --worker host1,host2,host3

# Worker node
./juicefs-sync-advanced sync --manager manager-addr:1234
```

## Database Schema

### `objects` table (sync records)

Tracks the status of each transferred object: `InTransfer`, `Transferred`, `Verified`, `Skipped`, `Error`.

### `sync_plan` table (dry-run plan)

Records planned actions during `--dry` mode:
- `Copy` / `Skip` / `Update` / `DeleteSrc` / `DeleteDst` / `Checksum` / `CopyPerms`
- Includes per-file `scanned_at` timestamp for performance analysis

### `sync_runs` table (performance summary)

Auto-generated at the end of each run with aggregate statistics:
`total_scanned`, `copied`, `skipped`, `extra`, `deleted`, `elapsed_ms`, etc.

## `scan` Command — Object Inventory

Scan a single bucket's metadata (key, size, mtime, storage class) into a database or CSV, with time-range filtering.

```bash
# Scan all objects to SQLite
./juicefs-sync-advanced scan --db-type sqlite3 --db-dsn /tmp/inv.db s3://mybucket/

# Scan with time range + CSV export
./juicefs-sync-advanced scan --start "2025-01-01" --end "2025-06-30" --export inv.csv s3://mybucket/

# Re-export existing scan_id to CSV
./juicefs-sync-advanced scan --db-type sqlite3 --db-dsn /tmp/inv.db --scan-id scan-xxx --export out.csv
```

### scan flags

| Flag | Description |
|------|-------------|
| `--db-type` | Database type: `mysql`, `postgres`, `sqlite3` |
| `--db-dsn` | Database DSN |
| `--prefix` | Only scan objects with the given prefix |
| `--start` | Only include objects modified after this time |
| `--end` | Only include objects modified before this time |
| `--limit N` | Stop after N objects |
| `--export FILE` | Save results to CSV |
| `--scan-id ID` | Scan run ID (auto-generated if not set) |
| `--no-https` | Use HTTP instead of HTTPS (for Ceph/MinIO) |

### `object_inventory` table

```sql
CREATE TABLE object_inventory (
    scan_id VARCHAR(64) NOT NULL,
    key VARCHAR(1024) NOT NULL,
    size BIGINT,
    mtime DATETIME,
    storage_class VARCHAR(32),
    is_dir INT,
    scanned_at DATETIME,
    PRIMARY KEY (scan_id, key)
);
```

### Scan diff: compare two runs

Compare two scan runs to find what changed — useful for migration validation.

```bash
juicefs-sync-advanced scan \
  --db-type sqlite3 --db-dsn /tmp/demo.db \
  --diff-from pre-migration --diff-to post-migration \
  --export /tmp/diff.csv
```

Output: CSV with `change` column — `+` (new), `-` (deleted), `~` (modified).

---

## License

Apache License 2.0 — see [LICENSE](LICENSE).
