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

	// ETag capture via optional interface
	hasETag := false
	if cfg.WithHead {
		if _, ok := src.(object.ObjectWithETag); ok {
			hasETag = true
		}
	}

	// open CSV writer
	var csvWriter *csv.Writer
	var csvFile *os.File
	var csvErr error
	csvHeader := []string{"key", "size", "mtime", "storage_class", "is_dir"}
	if hasETag {
		csvHeader = []string{"key", "size", "mtime", "storage_class", "etag", "is_dir"}
	}
	if cfg.Export != "" {
		var err error
		csvFile, err = os.Create(cfg.Export)
		if err != nil {
			return fmt.Errorf("create csv file: %w", err)
		}
		defer csvFile.Close()
		csvWriter = csv.NewWriter(csvFile)
		defer csvWriter.Flush()
		if err := csvWriter.Write(csvHeader); err != nil {
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

		etag := ""
		if hasETag {
			if o, ok := obj.(object.ObjectWithETag); ok {
				etag = o.ETag()
			}
		}

		rec := &InventoryRecord{
			ScanID:       scanID,
			Key:          obj.Key(),
			Size:         obj.Size(),
			Mtime:        mtime,
			StorageClass: obj.StorageClass(),
			ETag:         etag,
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
			var row []string
			if hasETag {
				row = []string{rec.Key, fmt.Sprintf("%d", rec.Size), mtimeStr,
					rec.StorageClass, rec.ETag, "false"}
			} else {
				row = []string{rec.Key, fmt.Sprintf("%d", rec.Size), mtimeStr,
					rec.StorageClass, "false"}
			}
			if err := csvWriter.Write(row); err != nil && csvErr == nil {
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

// DiffRecord represents a single difference between two inventory scans.
type DiffRecord struct {
	Change string // "+" new, "-" deleted, "~" modified
	Key    string
	SizeA  int64
	MtimeA string
	SizeB  int64
	MtimeB string
}

// DiffScans compares two scan_ids and writes the diff to a CSV file.
func DiffScans(dbType, dsn, scanIDA, scanIDB, output string) (int, error) {
	m, err := openInventoryDB(dbType, dsn)
	if err != nil {
		return 0, err
	}
	defer m.close()

	loadScan := func(scanID string) (map[string]*InventoryRecord, error) {
		result := make(map[string]*InventoryRecord)
		var query string
		if m.dbType == "postgres" {
			query = "SELECT `key`, size, mtime, storage_class, is_dir FROM object_inventory WHERE scan_id = $1"
		} else {
			query = "SELECT `key`, size, mtime, storage_class, is_dir FROM object_inventory WHERE scan_id = ?"
		}
		rows, err := m.db.Query(query, scanID)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		for rows.Next() {
			var key, sc string
			var size int64
			var mtime sql.NullTime
			var isDir int
			if err := rows.Scan(&key, &size, &mtime, &sc, &isDir); err != nil {
				return nil, err
			}
			result[key] = &InventoryRecord{
				ScanID: scanID, Key: key, Size: size,
				Mtime: mtime.Time, StorageClass: sc, IsDir: isDir == 1,
			}
		}
		return result, rows.Err()
	}

	scanA, err := loadScan(scanIDA)
	if err != nil {
		return 0, fmt.Errorf("load scan %s: %w", scanIDA, err)
	}
	scanB, err := loadScan(scanIDB)
	if err != nil {
		return 0, fmt.Errorf("load scan %s: %w", scanIDB, err)
	}

	f, err := os.Create(output)
	if err != nil {
		return 0, fmt.Errorf("create csv: %w", err)
	}
	defer f.Close()
	w := csv.NewWriter(f)
	defer w.Flush()

	if err := w.Write([]string{"change", "key", "size_a", "mtime_a", "size_b", "mtime_b"}); err != nil {
		return 0, err
	}

	count := 0

	// Keys in B but not in A → added
	// Keys in A but not in B → deleted
	// Keys in both with different size → modified
	for key, recB := range scanB {
		if recA, ok := scanA[key]; ok {
			if recA.Size != recB.Size || !recA.Mtime.Equal(recB.Mtime) {
				mta := ""
				if !recA.Mtime.IsZero() {
					mta = recA.Mtime.Format(time.RFC3339)
				}
				mtb := ""
				if !recB.Mtime.IsZero() {
					mtb = recB.Mtime.Format(time.RFC3339)
				}
				_ = w.Write([]string{"~", key,
					fmt.Sprintf("%d", recA.Size), mta,
					fmt.Sprintf("%d", recB.Size), mtb})
				count++
			}
		} else {
			mtb := ""
			if !recB.Mtime.IsZero() {
				mtb = recB.Mtime.Format(time.RFC3339)
			}
			_ = w.Write([]string{"+", key, "0", "", fmt.Sprintf("%d", recB.Size), mtb})
			count++
		}
	}
	for key, recA := range scanA {
		if _, ok := scanB[key]; !ok {
			mta := ""
			if !recA.Mtime.IsZero() {
				mta = recA.Mtime.Format(time.RFC3339)
			}
			_ = w.Write([]string{"-", key, fmt.Sprintf("%d", recA.Size), mta, "0", ""})
			count++
		}
	}

	logger.Infof("Diff complete: %d differences (scanA=%s scanB=%s output=%s)", count, scanIDA, scanIDB, output)
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
