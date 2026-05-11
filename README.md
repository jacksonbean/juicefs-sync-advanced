# juicefs-sync-approve

**A streamlined object storage sync tool** — extracted from [JuiceFS](https://github.com/juicedata/juicefs), focused solely on the `sync` command with enhanced dry-run, database recording, and performance analysis capabilities.

## Features

- **Multi-cloud sync** — sync objects between S3, OSS, COS, GCS, Azure Blob, local filesystem, and 30+ storage backends
- **Dry-run mode** (`--dry`) — scan and compare source/destination without copying, records planned actions to database
- **Database recording** — record scan results to MySQL, PostgreSQL, or SQLite for analysis
- **Plan table** — see exactly what would be copied, skipped, deleted, or updated before running
- **Performance metrics** — Prometheus metrics + `sync_runs` summary table for migration performance benchmarking
- **Include/exclude filters** — rsync-compatible pattern matching
- **Cluster mode** — distributed sync with manager/worker architecture
- **Checksum verification** — optional end-to-end data integrity checking
- **Bandwidth limiting** — per-process and global traffic control

## Quick Start

### Build

```bash
go build -o juicefs-sync .
```

### Sync between two S3 buckets

```bash
./juicefs-sync sync s3://bucket1/path/ s3://bucket2/path/ -p 20
```

### Dry-run with SQLite recording

```bash
./juicefs-sync sync \
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
juicefs-sync sync [command options] SRC DST
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
./juicefs-sync sync --include='logs/**' --exclude='temp/**' s3://src/ /mnt/dst/
```

**Migration analysis with Postgres:**
```bash
./juicefs-sync sync --dry \
  --record-db-type postgres \
  --record-db-dsn "host=localhost user=app dbname=sync_analysis" \
  --record-plan-table migration_2025 \
  s3://old-bucket/ s3://new-bucket/
```

**Cluster mode (distributed sync):**
```bash
# Manager node
./juicefs-sync sync s3://src/ s3://dst/ --worker host1,host2,host3

# Worker node
./juicefs-sync sync --manager manager-addr:1234
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

## License

Apache License 2.0 — see [LICENSE](LICENSE).
