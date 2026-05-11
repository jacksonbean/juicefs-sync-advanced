package scan

import (
	"encoding/csv"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/juicedata/juicefs/pkg/object"
	"github.com/juicedata/juicefs/pkg/sync"
	"github.com/juicedata/juicefs/pkg/utils"
)

var scannedCount atomic.Int64

func Run(src object.ObjectStorage, cfg *Config) error {
	scanID := cfg.ScanID
	if scanID == "" {
		scanID = fmt.Sprintf("scan-%d", time.Now().UnixNano())
	}

	scannedAt := time.Now()
	startKey := ""
	if cfg.StartTime.IsZero() && cfg.EndTime.IsZero() {
		logger.Infof("Scanning all objects from %s", src)
	} else {
		logger.Infof("Scanning objects from %s (start=%s end=%s)",
			src, cfg.StartTime.Format(time.RFC3339), cfg.EndTime.Format(time.RFC3339))
	}

	// open database if configured
	var db *inventoryManager
	if cfg.DBType != "" && cfg.DBDSN != "" {
		var err error
		db, err = openInventoryDB(cfg.DBType, cfg.DBDSN)
		if err != nil {
			return fmt.Errorf("open inventory db: %w", err)
		}
		defer db.close()
	}

	// open CSV writer if configured
	var csvWriter *csv.Writer
	var csvFile *os.File
	if cfg.Export != "" {
		var err error
		csvFile, err = os.Create(cfg.Export)
		if err != nil {
			return fmt.Errorf("create csv file: %w", err)
		}
		defer csvFile.Close()
		csvWriter = csv.NewWriter(csvFile)
		defer csvWriter.Flush()
		// header
		if err := csvWriter.Write([]string{"key", "size", "mtime", "storage_class", "is_dir"}); err != nil {
			return fmt.Errorf("write csv header: %w", err)
		}
	}

	// progress bar
	progress := utils.NewProgress(false)
	bar := progress.AddCountBar("Scanned objects", 0)

	// list objects
	objChan, err := sync.ListAll(src, cfg.Prefix, startKey, "", false)
	if err != nil {
		return fmt.Errorf("list objects: %w", err)
	}

	for obj := range objChan {
		if obj == nil {
			break
		}
		if obj.IsDir() {
			continue
		}

		// filter by time range
		mtime := obj.Mtime()
		if !cfg.StartTime.IsZero() && mtime.Before(cfg.StartTime) {
			continue
		}
		if !cfg.EndTime.IsZero() && mtime.After(cfg.EndTime) {
			// objects are sorted by key, not by mtime, so we can't break
			continue
		}

		rec := &InventoryRecord{
			ScanID:       scanID,
			Key:          obj.Key(),
			Size:         obj.Size(),
			Mtime:        mtime,
			StorageClass: obj.StorageClass(),
			IsDir:        false,
			ScannedAt:    scannedAt,
		}

		// write to database
		if db != nil {
			if err := db.insertRecord(rec); err != nil {
				logger.Warnf("insert record for %s: %s", rec.Key, err)
			}
		}

		// write to CSV
		if csvWriter != nil {
			mtimeStr := ""
			if !mtime.IsZero() {
				mtimeStr = mtime.Format(time.RFC3339)
			}
			_ = csvWriter.Write([]string{
				rec.Key,
				fmt.Sprintf("%d", rec.Size),
				mtimeStr,
				rec.StorageClass,
				"false",
			})
		}

		n := scannedCount.Add(1)
		bar.SetTotal(n)
		bar.Increment()
	}

	bar.Done()
	total := scannedCount.Load()
	logger.Infof("Scan complete: %d objects scanned (scan_id=%s)", total, scanID)

	if csvFile != nil {
		logger.Infof("CSV exported to %s", cfg.Export)
	}
	return nil
}

// ExportCSV exports a scan run from database to CSV file.
// Returns count of exported rows.
func ExportCSV(dbType, dsn, scanID, output string) (int, error) {
	m, err := openInventoryDB(dbType, dsn)
	if err != nil {
		return 0, err
	}
	defer m.close()

	f, err := os.Create(output)
	if err != nil {
		return 0, fmt.Errorf("create csv: %w", err)
	}
	defer f.Close()

	w := csv.NewWriter(f)
	defer w.Flush()

	if err := w.Write([]string{"key", "size", "mtime", "storage_class", "is_dir"}); err != nil {
		return 0, err
	}

	var query string
	var args []interface{}

	if m.dbType == "mysql" || m.dbType == "sqlite3" {
		if scanID != "" {
			query = "SELECT `key`, size, mtime, storage_class, is_dir FROM object_inventory WHERE scan_id = ? ORDER BY `key`"
			args = []interface{}{scanID}
		} else {
			query = "SELECT `key`, size, mtime, storage_class, is_dir FROM object_inventory ORDER BY scan_id, `key`"
		}
	} else {
		if scanID != "" {
			query = "SELECT key, size, mtime, storage_class, is_dir FROM object_inventory WHERE scan_id = $1 ORDER BY key"
			args = []interface{}{scanID}
		} else {
			query = "SELECT key, size, mtime, storage_class, is_dir FROM object_inventory ORDER BY scan_id, key"
		}
	}

	rows, err := m.db.Query(query, args...)
	if err != nil {
		return 0, fmt.Errorf("query: %w", err)
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var key, storageClass string
		var size int64
		var mtime sqlTime
		var isDir int
		if err := rows.Scan(&key, &size, &mtime, &storageClass, &isDir); err != nil {
			return 0, fmt.Errorf("scan row: %w", err)
		}
		mtimeStr := ""
		if mtime.Valid {
			mtimeStr = mtime.Time.Format(time.RFC3339)
		}
		_ = w.Write([]string{
			key,
			fmt.Sprintf("%d", size),
			mtimeStr,
			storageClass,
			fmt.Sprintf("%t", isDir == 1),
		})
		count++
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	return count, nil
}

// sqlTime wraps sql.NullTime for scanning
type sqlTime struct {
	Time  time.Time
	Valid bool
}

func (t *sqlTime) Scan(value interface{}) error {
	if value == nil {
		t.Valid = false
		return nil
	}
	switch v := value.(type) {
	case time.Time:
		t.Time = v
		t.Valid = true
	case string:
		if strings.TrimSpace(v) == "" {
			t.Valid = false
			return nil
		}
		var err error
		t.Time, err = time.Parse("2006-01-02 15:04:05", v)
		if err != nil {
			t.Time, err = time.Parse(time.RFC3339, v)
			if err != nil {
				t.Time, err = time.Parse("2006-01-02T15:04:05Z", v)
			}
		}
		t.Valid = err == nil
		if !t.Valid {
			t.Valid = true
			t.Time, _ = time.Parse("2006-01-02 15:04:05 -0700", strings.ReplaceAll(v, "T", " "))
		}
	}
	return nil
}
