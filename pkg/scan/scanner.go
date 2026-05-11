package scan

import (
	"database/sql"
	"encoding/csv"
	"fmt"
	"os"
	"time"

	"github.com/jacksonbean/juicefs-sync-advanced/pkg/object"
	"github.com/jacksonbean/juicefs-sync-advanced/pkg/sync"
	"github.com/jacksonbean/juicefs-sync-advanced/pkg/utils"
)

func Run(src object.ObjectStorage, cfg *Config) error {
	var scannedCount int64
	scanID := cfg.ScanID
	if scanID == "" {
		scanID = fmt.Sprintf("scan-%d", time.Now().UnixNano())
	}
	scannedAt := time.Now()

	if cfg.StartTime.IsZero() && cfg.EndTime.IsZero() {
		logger.Infof("Scanning all objects from %s", src)
	} else {
		logger.Infof("Scanning objects from %s (start=%s end=%s)",
			src, cfg.StartTime.Format(time.RFC3339), cfg.EndTime.Format(time.RFC3339))
	}

	// open database
	var db *inventoryManager
	if cfg.DBType != "" && cfg.DBDSN != "" {
		var err error
		db, err = openInventoryDB(cfg.DBType, cfg.DBDSN)
		if err != nil {
			return fmt.Errorf("open inventory db: %w", err)
		}
		defer db.close()
	}

	// open CSV writer
	var csvWriter *csv.Writer
	var csvFile *os.File
	var csvErr error
	if cfg.Export != "" {
		var err error
		csvFile, err = os.Create(cfg.Export)
		if err != nil {
			return fmt.Errorf("create csv file: %w", err)
		}
		defer csvFile.Close()
		csvWriter = csv.NewWriter(csvFile)
		defer csvWriter.Flush()
		if err := csvWriter.Write([]string{"key", "size", "mtime", "storage_class", "is_dir"}); err != nil {
			return fmt.Errorf("write csv header: %w", err)
		}
	}

	progress := utils.NewProgress(false)
	spinner := progress.AddCountSpinner("Scanned objects")

	objChan, err := sync.ListAll(src, cfg.Prefix, "", "", false)
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
		if cfg.Limit > 0 && scannedCount >= cfg.Limit {
			break
		}

		mtime := obj.Mtime()
		if !cfg.StartTime.IsZero() && mtime.Before(cfg.StartTime) {
			continue
		}
		if !cfg.EndTime.IsZero() && mtime.After(cfg.EndTime) {
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

		if db != nil {
			if err := db.insertRecord(rec); err != nil {
				logger.Warnf("insert record for %s: %s", rec.Key, err)
			}
		}

		if csvWriter != nil {
			mtimeStr := ""
			if !mtime.IsZero() {
				mtimeStr = mtime.Format(time.RFC3339)
			}
			if err := csvWriter.Write([]string{
				rec.Key, fmt.Sprintf("%d", rec.Size), mtimeStr,
				rec.StorageClass, "false",
			}); err != nil && csvErr == nil {
				csvErr = err
			}
		}

		scannedCount++
		spinner.Increment()
	}

	spinner.Done()
	logger.Infof("Scan complete: %d objects scanned (scan_id=%s)", scannedCount, scanID)

	if csvFile != nil {
		if csvErr != nil {
			return fmt.Errorf("csv write error: %w", csvErr)
		}
		logger.Infof("CSV exported to %s", cfg.Export)
	}
	return nil
}

// ExportCSV exports a scan run from database to CSV file, optionally filtered by time range.
func ExportCSV(dbType, dsn, scanID, startTime, endTime, output string) (int, error) {
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

	// Build query with optional time range filters
	var query string
	var args []interface{}

	where := ""
	params := []string{}

	if scanID != "" {
		if m.dbType == "postgres" {
			params = append(params, fmt.Sprintf("scan_id = $%d", len(params)+1))
		} else {
			params = append(params, "scan_id = ?")
		}
		args = append(args, scanID)
	}
	if startTime != "" {
		if m.dbType == "postgres" {
			params = append(params, fmt.Sprintf("mtime >= $%d", len(params)+1))
		} else {
			params = append(params, "mtime >= ?")
		}
		args = append(args, startTime)
	}
	if endTime != "" {
		if m.dbType == "postgres" {
			params = append(params, fmt.Sprintf("mtime <= $%d", len(params)+1))
		} else {
			params = append(params, "mtime <= ?")
		}
		args = append(args, endTime)
	}

	if len(params) > 0 {
		where = " WHERE " + stringsJoin(params, " AND ")
	}
	orderBy := " ORDER BY `key`"
	if m.dbType == "postgres" {
		orderBy = " ORDER BY key"
	}
	query = fmt.Sprintf("SELECT `key`, size, mtime, storage_class, is_dir FROM object_inventory%s%s", where, orderBy)
	if m.dbType == "postgres" {
		query = fmt.Sprintf("SELECT key, size, mtime, storage_class, is_dir FROM object_inventory%s%s", where, orderBy)
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
		var mtime sql.NullTime
		var isDir int
		if err := rows.Scan(&key, &size, &mtime, &storageClass, &isDir); err != nil {
			return 0, fmt.Errorf("scan row: %w", err)
		}
		mtimeStr := ""
		if mtime.Valid {
			mtimeStr = mtime.Time.Format(time.RFC3339)
		}
		if err := w.Write([]string{
			key, fmt.Sprintf("%d", size), mtimeStr, storageClass,
			fmt.Sprintf("%t", isDir == 1),
		}); err != nil {
			return 0, fmt.Errorf("write csv row: %w", err)
		}
		count++
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	return count, nil
}

func stringsJoin(elems []string, sep string) string {
	out := ""
	for i, e := range elems {
		if i > 0 {
			out += sep
		}
		out += e
	}
	return out
}
