package scan

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/jacksonbean/juicefs-sync-advanced/pkg/utils"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"
	_ "github.com/mattn/go-sqlite3"
)

var logger = utils.GetLogger("juicefs")

type InventoryRecord struct {
	ScanID       string
	Key          string
	Size         int64
	Mtime        time.Time
	StorageClass string
	ETag         string
	IsDir        bool
	ScannedAt    time.Time
}

type inventoryManager struct {
	db      *sql.DB
	dbType  string
	scanID  string
}

func openInventoryDB(dbType, dsn string) (*inventoryManager, error) {
	dbType = strings.ToLower(dbType)
	var db *sql.DB
	var err error
	switch dbType {
	case "mysql":
		db, err = sql.Open("mysql", dsn)
	case "postgres", "postgresql":
		db, err = sql.Open("pgx", dsn)
	case "sqlite3":
		db, err = sql.Open("sqlite3", dsn)
	default:
		return nil, fmt.Errorf("unsupported database type: %s", dbType)
	}
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	if err = db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	m := &inventoryManager{db: db, dbType: dbType}
	if err = m.createTable(); err != nil {
		db.Close()
		return nil, fmt.Errorf("create inventory table: %w", err)
	}
	return m, nil
}

func (m *inventoryManager) close() error {
	return m.db.Close()
}

func (m *inventoryManager) createTable() error {
	var pk string
	if m.dbType == "mysql" {
		pk = "PRIMARY KEY (scan_id, `key`(700))"
	} else {
		pk = "PRIMARY KEY (scan_id, key)"
	}
	createSQL := fmt.Sprintf(`CREATE TABLE IF NOT EXISTS object_inventory (
		scan_id VARCHAR(64) NOT NULL,
		key VARCHAR(1024) NOT NULL,
		size BIGINT,
		mtime DATETIME,
		storage_class VARCHAR(32),
		etag VARCHAR(64),
		is_dir INT,
		scanned_at DATETIME,
		%s
	)`, pk)
	if m.dbType == "mysql" {
		createSQL += " ENGINE = InnoDB"
	}
	_, err := m.db.Exec(createSQL)
	if err != nil {
		return fmt.Errorf("exec create: %w", err)
	}

	// composite index for time-range queries
	var idxSQL string
	if m.dbType == "postgres" || m.dbType == "sqlite3" {
		idxSQL = fmt.Sprintf("CREATE INDEX IF NOT EXISTS idx_inv_scan_mtime ON object_inventory (scan_id, mtime)")
	} else {
		idxSQL = fmt.Sprintf("CREATE INDEX idx_inv_scan_mtime ON object_inventory (scan_id, mtime)")
	}
	_, _ = m.db.Exec(idxSQL)

	return nil
}

func nullTime(t time.Time) sql.NullTime {
	if t.IsZero() {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: t, Valid: true}
}

func (m *inventoryManager) insertRecord(r *InventoryRecord) error {
	isDir := 0
	if r.IsDir {
		isDir = 1
	}

	switch m.dbType {
	case "mysql":
		query := `INSERT INTO object_inventory (scan_id, key, size, mtime, storage_class, etag, is_dir, scanned_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)
			ON DUPLICATE KEY UPDATE
			size = VALUES(size), mtime = VALUES(mtime),
			storage_class = VALUES(storage_class), etag = VALUES(etag),
			is_dir = VALUES(is_dir), scanned_at = VALUES(scanned_at)`
		_, err := m.db.Exec(query, r.ScanID, r.Key, r.Size, nullTime(r.Mtime), r.StorageClass, r.ETag, isDir, nullTime(r.ScannedAt))
		return err
	case "sqlite3":
		query := `INSERT INTO object_inventory (scan_id, key, size, mtime, storage_class, etag, is_dir, scanned_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT (scan_id, key) DO UPDATE SET
			size = EXCLUDED.size, mtime = EXCLUDED.mtime,
			storage_class = EXCLUDED.storage_class, etag = EXCLUDED.etag,
			is_dir = EXCLUDED.is_dir, scanned_at = EXCLUDED.scanned_at`
		_, err := m.db.Exec(query, r.ScanID, r.Key, r.Size, nullTime(r.Mtime), r.StorageClass, r.ETag, isDir, nullTime(r.ScannedAt))
		return err
	default: // postgres
		query := `INSERT INTO object_inventory (scan_id, key, size, mtime, storage_class, etag, is_dir, scanned_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			ON CONFLICT (scan_id, key) DO UPDATE SET
			size = EXCLUDED.size, mtime = EXCLUDED.mtime,
			storage_class = EXCLUDED.storage_class, etag = EXCLUDED.etag,
			is_dir = EXCLUDED.is_dir, scanned_at = EXCLUDED.scanned_at`
		_, err := m.db.Exec(query, r.ScanID, r.Key, r.Size, nullTime(r.Mtime), r.StorageClass, r.ETag, isDir, nullTime(r.ScannedAt))
		return err
	}
}
