/*
 * JuiceFS, Copyright 2018 Juicedata, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package record

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/jacksonbean/juicefs-sync-advanced/pkg/utils"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"
	_ "github.com/mattn/go-sqlite3"
)

const (
	StatusInTransfer    = "InTransfer"
	StatusSkipped       = "Skipped"
	StatusTransferred   = "Transferred"
	StatusVerified      = "Verified"
	StatusError         = "Error"
	StatusSourceDeleted = "SourceDeleted"
)

var tableNameRegexp = regexp.MustCompile(`^[a-zA-Z0-9_]+$`)
var logger = utils.GetLogger("juicefs")

func safeTruncate(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	end := maxBytes
	for end > 0 {
		b := s[end-1]
		if b&0xC0 == 0x80 {
			end--
		} else if b&0xC0 == 0xC0 {
			end--
			break
		} else {
			break
		}
	}
	return s[:end]
}

func validateTableName(name string) error {
	if name == "" {
		return fmt.Errorf("table name cannot be empty")
	}
	if !tableNameRegexp.MatchString(name) {
		return fmt.Errorf("invalid table name %q: only letters, digits and underscore allowed", name)
	}
	return nil
}

func nullTime(t time.Time) sql.NullTime {
	if t.IsZero() {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: t, Valid: true}
}

type SyncRecord struct {
	SourceID               string    `json:"source_id"`
	TargetID               string    `json:"target_id"`
	IsDirectory            bool      `json:"is_directory"`
	Size                   int64     `json:"size"`
	Mtime                  time.Time `json:"mtime"`
	Status                 string    `json:"status"`
	TransferStart          time.Time `json:"transfer_start,omitempty"`
	TransferComplete       time.Time `json:"transfer_complete,omitempty"`
	VerifyStart            time.Time `json:"verify_start,omitempty"`
	VerifyComplete         time.Time `json:"verify_complete,omitempty"`
	RetryCount             int       `json:"retry_count"`
	ErrorMessage           string    `json:"error_message,omitempty"`
	IsSourceDeleted        bool      `json:"is_source_deleted"`
	SourceMD5              string    `json:"source_md5,omitempty"`
	SourceRetentionEndTime time.Time `json:"source_retention_end_time,omitempty"`
	TargetMtime            time.Time `json:"target_mtime,omitempty"`
	TargetMD5              string    `json:"target_md5,omitempty"`
	TargetRetentionEndTime time.Time `json:"target_retention_end_time,omitempty"`
	FirstErrorMessage      string    `json:"first_error_message,omitempty"`
}

type PlanRecord struct {
	RunID     string    `json:"run_id"`
	SourceID  string    `json:"source_id"`
	TargetID  string    `json:"target_id"`
	Size      int64     `json:"size"`
	Mtime     time.Time `json:"mtime"`
	Action    string    `json:"action"`
	Reason    string    `json:"reason"`
	ScannedAt time.Time `json:"scanned_at"`
}

type RunSummary struct {
	RunID        string
	Src          string
	Dst          string
	Dry          bool
	TotalScanned int64
	TotalBytes   int64
	Copied       int64
	CopiedBytes  int64
	Skipped      int64
	Extra        int64
	Deleted      int64
	Failed       int64
	StartedAt    time.Time
	FinishedAt   time.Time
	ConfigJSON   string
}

type Recorder interface {
	Record(r *SyncRecord) error
	RecordPlan(p *PlanRecord) error
	RecordSummary(s *RunSummary) error
	Close() error
}

type ManagerConfig struct {
	TableName             string
	ExtendedFieldsEnabled bool
	MaxErrorSize          int
	BatchSize             int
	FlushInterval         time.Duration
	MaxOpenConns          int
	MaxIdleConns          int
	DryRun                bool
	PlanTableName         string
}

type manager struct {
	db                    *sql.DB
	tableName             string
	dbType                string
	extendedFieldsEnabled bool
	planTableName         string
	maxErrorSize          int
}

func NewRecorder(dbType, dsn string, cfg *ManagerConfig) (Recorder, error) {
	if cfg == nil {
		cfg = &ManagerConfig{}
	}
	if cfg.TableName == "" {
		cfg.TableName = "objects"
	}
	if cfg.PlanTableName == "" {
		cfg.PlanTableName = "sync_plan"
	}
	if cfg.MaxErrorSize == 0 {
		cfg.MaxErrorSize = 2048
	}
	if err := validateTableName(cfg.TableName); err != nil {
		return nil, err
	}

	var db *sql.DB
	var err error

	dbType = strings.ToLower(dbType)
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
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if cfg.MaxOpenConns > 0 {
		db.SetMaxOpenConns(cfg.MaxOpenConns)
	}
	if cfg.MaxIdleConns > 0 {
		db.SetMaxIdleConns(cfg.MaxIdleConns)
	}

	if err = db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	m := &manager{
		db:                    db,
		tableName:             cfg.TableName,
		planTableName:         cfg.PlanTableName,
		dbType:                dbType,
		extendedFieldsEnabled: cfg.ExtendedFieldsEnabled,
		maxErrorSize:          cfg.MaxErrorSize,
	}
	if err = m.createTable(); err != nil {
		m.Close()
		return nil, fmt.Errorf("failed to create table: %w", err)
	}
	if err = m.alterTableIfNeeded(); err != nil {
		m.Close()
		return nil, fmt.Errorf("failed to alter table: %w", err)
	}
	if cfg.DryRun {
		if err = m.createPlanTable(); err != nil {
			m.Close()
			return nil, fmt.Errorf("failed to create plan table: %w", err)
		}
	}

	if cfg.BatchSize > 1 {
		return newBatchRecorder(m, cfg.BatchSize, cfg.FlushInterval), nil
	}
	return m, nil
}

func (m *manager) createTable() error {
	errorSize := m.maxErrorSize
	if errorSize < 2048 {
		errorSize = 2048
	}

	baseColumns := fmt.Sprintf(`source_id VARCHAR(1024) NOT NULL,
    target_id VARCHAR(1024) NULL,
    is_directory INT NOT NULL,
    size BIGINT NULL,
    mtime DATETIME NULL,
    status VARCHAR(32) NOT NULL,
    transfer_start DATETIME NULL,
    transfer_complete DATETIME NULL,
    verify_start DATETIME NULL,
    verify_complete DATETIME NULL,
    retry_count INT NULL,
    error_message VARCHAR(%d) NULL,
    is_source_deleted INT NULL`, errorSize)

	var extraColumns string
	if m.extendedFieldsEnabled {
		extraColumns = fmt.Sprintf(`, source_md5 VARCHAR(32) NULL,
    source_retention_end_time DATETIME NULL,
    target_mtime DATETIME NULL,
    target_md5 VARCHAR(32) NULL,
    target_retention_end_time DATETIME NULL,
    first_error_message VARCHAR(%d) NULL`, errorSize)
	}

	var pkClause string
	if m.dbType == "mysql" {
		pkClause = ", PRIMARY KEY (source_id(768))"
	} else {
		pkClause = ", PRIMARY KEY (source_id)"
	}

	createSQL := "CREATE TABLE IF NOT EXISTS " + m.tableName + " (" + baseColumns + extraColumns + pkClause + ")"
	if m.dbType == "mysql" {
		createSQL += " ENGINE = InnoDB ROW_FORMAT = COMPRESSED"
	}

	if _, err := m.db.Exec(createSQL); err != nil {
		return fmt.Errorf("create table failed: %w", err)
	}

	if m.dbType == "postgres" || m.dbType == "sqlite3" {
		idxSQL := "CREATE INDEX IF NOT EXISTS status_idx ON " + m.tableName + " (status)"
		if _, err := m.db.Exec(idxSQL); err != nil {
			return fmt.Errorf("create index failed: %w", err)
		}
	} else {
		_, _ = m.db.Exec("CREATE INDEX status_idx ON " + m.tableName + " (status)")
	}
	return nil
}

func (m *manager) columnExists(column string) (bool, error) {
	var query string
	if m.dbType == "mysql" {
		query = "SELECT 1 FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = ? AND column_name = ?"
	} else if m.dbType == "sqlite3" {
		query = "SELECT 1 FROM pragma_table_info(?) WHERE name = ?"
	} else {
		query = "SELECT 1 FROM information_schema.columns WHERE table_name = $1 AND column_name = $2"
	}
	var dummy int
	err := m.db.QueryRow(query, m.tableName, column).Scan(&dummy)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (m *manager) alterTableIfNeeded() error {
	if !m.extendedFieldsEnabled {
		return nil
	}

	columns := []struct {
		name string
		def  string
	}{
		{"source_md5", "VARCHAR(32)"},
		{"source_retention_end_time", "DATETIME"},
		{"target_mtime", "DATETIME"},
		{"target_md5", "VARCHAR(32)"},
		{"target_retention_end_time", "DATETIME"},
		{"first_error_message", fmt.Sprintf("VARCHAR(%d)", m.maxErrorSize)},
	}

	for _, col := range columns {
		exists, err := m.columnExists(col.name)
		if err != nil {
			return fmt.Errorf("check column %s existence failed: %w", col.name, err)
		}
		if exists {
			continue
		}
		sql := fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", m.tableName, col.name, col.def)
		if _, err := m.db.Exec(sql); err != nil {
			return fmt.Errorf("add column %s failed: %w", col.name, err)
		}
	}
	return nil
}

func (m *manager) buildQueryAndArgs(r *SyncRecord) (string, []interface{}) {
	errMsg := safeTruncate(r.ErrorMessage, m.maxErrorSize)
	firstErrMsg := safeTruncate(r.FirstErrorMessage, m.maxErrorSize)

	isDir := 0
	if r.IsDirectory {
		isDir = 1
	}
	isSrcDel := 0
	if r.IsSourceDeleted {
		isSrcDel = 1
	}

	baseColumns := []string{"source_id", "target_id", "is_directory", "size", "mtime", "status",
		"transfer_start", "transfer_complete", "verify_start", "verify_complete", "retry_count",
		"error_message", "is_source_deleted"}
	baseArgs := []interface{}{
		r.SourceID, r.TargetID, isDir, r.Size, nullTime(r.Mtime), r.Status,
		nullTime(r.TransferStart), nullTime(r.TransferComplete), nullTime(r.VerifyStart), nullTime(r.VerifyComplete), r.RetryCount,
		errMsg, isSrcDel,
	}

	var extColumns []string
	var extArgs []interface{}
	if m.extendedFieldsEnabled {
		extColumns = []string{"source_md5", "source_retention_end_time", "target_mtime", "target_md5", "target_retention_end_time", "first_error_message"}
		extArgs = []interface{}{
			r.SourceMD5, nullTime(r.SourceRetentionEndTime), nullTime(r.TargetMtime),
			r.TargetMD5, nullTime(r.TargetRetentionEndTime), firstErrMsg,
		}
	}

	allColumns := append(baseColumns, extColumns...)
	args := append(baseArgs, extArgs...)

	if m.dbType == "mysql" {
		placeholders := strings.Repeat("?,", len(allColumns))
		placeholders = placeholders[:len(placeholders)-1]
		query := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s) ON DUPLICATE KEY UPDATE ",
			m.tableName, strings.Join(allColumns, ", "), placeholders)

		var updateParts []string
		for _, col := range baseColumns {
			if col == "source_id" {
				continue
			}
			updateParts = append(updateParts, fmt.Sprintf("%s = VALUES(%s)", col, col))
		}
		for _, col := range extColumns {
			if col == "first_error_message" {
				updateParts = append(updateParts, fmt.Sprintf("%s = COALESCE(%s, VALUES(%s))", col, col, col))
			} else {
				updateParts = append(updateParts, fmt.Sprintf("%s = VALUES(%s)", col, col))
			}
		}
		query += strings.Join(updateParts, ", ")
		return query, args
	}

	// Postgres uses $N placeholders, SQLite uses ? placeholders
	useDollar := m.dbType != "sqlite3"
	var placeholderStr string
	if useDollar {
		var ph []string
		for i := range allColumns {
			ph = append(ph, fmt.Sprintf("$%d", i+1))
		}
		placeholderStr = strings.Join(ph, ", ")
	} else {
		placeholderStr = strings.Repeat("?,", len(allColumns))
		placeholderStr = placeholderStr[:len(placeholderStr)-1]
	}

	query := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s) ON CONFLICT (source_id) DO UPDATE SET ",
		m.tableName, strings.Join(allColumns, ", "), placeholderStr)

	var updateParts []string
	for _, col := range baseColumns {
		if col == "source_id" {
			continue
		}
		updateParts = append(updateParts, fmt.Sprintf("%s = EXCLUDED.%s", col, col))
	}
	for _, col := range extColumns {
		if col == "first_error_message" {
			updateParts = append(updateParts, fmt.Sprintf("%s = COALESCE(%s.%s, EXCLUDED.%s)", col, m.tableName, col, col))
		} else {
			updateParts = append(updateParts, fmt.Sprintf("%s = EXCLUDED.%s", col, col))
		}
	}
	query += strings.Join(updateParts, ", ")
	return query, args
}

func (m *manager) Record(r *SyncRecord) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	query, args := m.buildQueryAndArgs(r)
	_, err := m.db.ExecContext(ctx, query, args...)
	return err
}

func (m *manager) createPlanTable() error {
	var pkClause string
	if m.dbType == "mysql" {
		pkClause = "PRIMARY KEY (run_id, source_id(700))"
	} else {
		pkClause = "PRIMARY KEY (run_id, source_id)"
	}
	createSQL := fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
		run_id VARCHAR(64) NOT NULL,
		source_id VARCHAR(1024) NOT NULL,
		target_id VARCHAR(1024),
		size BIGINT,
		mtime DATETIME,
		action VARCHAR(32) NOT NULL,
		reason VARCHAR(128),
		planned_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		scanned_at DATETIME,
		%s
	)`, m.planTableName, pkClause)
	if m.dbType == "mysql" {
		createSQL += " ENGINE = InnoDB"
	}
	if _, err := m.db.Exec(createSQL); err != nil {
		return fmt.Errorf("create plan table %s failed: %w", m.planTableName, err)
	}

	if m.dbType == "postgres" || m.dbType == "sqlite3" {
		idxSQL := fmt.Sprintf("CREATE INDEX IF NOT EXISTS idx_run_action ON %s (run_id, action)", m.planTableName)
		if _, err := m.db.Exec(idxSQL); err != nil {
			return fmt.Errorf("create plan table index failed: %w", err)
		}
	} else {
		_, _ = m.db.Exec(fmt.Sprintf("CREATE INDEX idx_run_action ON %s (run_id, action)", m.planTableName))
	}
	return nil
}

func (m *manager) buildPlanQueryAndArgs(p *PlanRecord) (string, []interface{}) {
	var query string
	switch m.dbType {
	case "mysql":
		query = fmt.Sprintf(`INSERT INTO %s (run_id, source_id, target_id, size, mtime, action, reason, planned_at, scanned_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, NOW(), ?)
			ON DUPLICATE KEY UPDATE
			target_id = VALUES(target_id),
			size = VALUES(size),
			mtime = VALUES(mtime),
			action = VALUES(action),
			reason = VALUES(reason),
			planned_at = VALUES(planned_at),
			scanned_at = VALUES(scanned_at)`, m.planTableName)
	case "sqlite3":
		query = fmt.Sprintf(`INSERT INTO %s (run_id, source_id, target_id, size, mtime, action, reason, planned_at, scanned_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, ?)
			ON CONFLICT (run_id, source_id) DO UPDATE SET
			target_id = EXCLUDED.target_id,
			size = EXCLUDED.size,
			mtime = EXCLUDED.mtime,
			action = EXCLUDED.action,
			reason = EXCLUDED.reason,
			planned_at = EXCLUDED.planned_at,
			scanned_at = EXCLUDED.scanned_at`, m.planTableName)
	default:
		query = fmt.Sprintf(`INSERT INTO %s (run_id, source_id, target_id, size, mtime, action, reason, planned_at, scanned_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, NOW(), $8)
			ON CONFLICT (run_id, source_id) DO UPDATE SET
			target_id = EXCLUDED.target_id,
			size = EXCLUDED.size,
			mtime = EXCLUDED.mtime,
			action = EXCLUDED.action,
			reason = EXCLUDED.reason,
			planned_at = EXCLUDED.planned_at,
			scanned_at = EXCLUDED.scanned_at`, m.planTableName)
	}
	args := []interface{}{p.RunID, p.SourceID, p.TargetID, p.Size, nullTime(p.Mtime), p.Action, p.Reason, nullTime(p.ScannedAt)}
	return query, args
}

func (m *manager) RecordPlan(p *PlanRecord) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	query, args := m.buildPlanQueryAndArgs(p)
	_, err := m.db.ExecContext(ctx, query, args...)
	return err
}

func (m *manager) exec(query string, args ...interface{}) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := m.db.ExecContext(ctx, query, args...)
	return err
}

func (m *manager) buildSummaryQueryAndArgs(s *RunSummary) (string, []interface{}) {
	dry := 0
	if s.Dry {
		dry = 1
	}
	elapsed := s.FinishedAt.Sub(s.StartedAt).Milliseconds()

	if m.dbType == "mysql" {
		query := `INSERT INTO sync_runs (run_id, src, dst, dry, total_scanned, total_bytes,
			copied, copied_bytes, skipped, extra, deleted, failed, started_at, finished_at, elapsed_ms, config_json)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON DUPLICATE KEY UPDATE
			src = VALUES(src), dst = VALUES(dst), dry = VALUES(dry),
			total_scanned = VALUES(total_scanned), total_bytes = VALUES(total_bytes),
			copied = VALUES(copied), copied_bytes = VALUES(copied_bytes),
			skipped = VALUES(skipped), extra = VALUES(extra),
			deleted = VALUES(deleted), failed = VALUES(failed),
			started_at = VALUES(started_at), finished_at = VALUES(finished_at),
			elapsed_ms = VALUES(elapsed_ms), config_json = VALUES(config_json)`
		args := []interface{}{s.RunID, s.Src, s.Dst, dry, s.TotalScanned, s.TotalBytes,
			s.Copied, s.CopiedBytes, s.Skipped, s.Extra, s.Deleted, s.Failed,
			s.StartedAt, s.FinishedAt, elapsed, s.ConfigJSON}
		return query, args
	}
	if m.dbType == "sqlite3" {
		query := `INSERT INTO sync_runs (run_id, src, dst, dry, total_scanned, total_bytes,
			copied, copied_bytes, skipped, extra, deleted, failed, started_at, finished_at, elapsed_ms, config_json)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT (run_id) DO UPDATE SET
			src = EXCLUDED.src, dst = EXCLUDED.dst, dry = EXCLUDED.dry,
			total_scanned = EXCLUDED.total_scanned, total_bytes = EXCLUDED.total_bytes,
			copied = EXCLUDED.copied, copied_bytes = EXCLUDED.copied_bytes,
			skipped = EXCLUDED.skipped, extra = EXCLUDED.extra,
			deleted = EXCLUDED.deleted, failed = EXCLUDED.failed,
			started_at = EXCLUDED.started_at, finished_at = EXCLUDED.finished_at,
			elapsed_ms = EXCLUDED.elapsed_ms, config_json = EXCLUDED.config_json`
		args := []interface{}{s.RunID, s.Src, s.Dst, dry, s.TotalScanned, s.TotalBytes,
			s.Copied, s.CopiedBytes, s.Skipped, s.Extra, s.Deleted, s.Failed,
			s.StartedAt, s.FinishedAt, elapsed, s.ConfigJSON}
		return query, args
	}
	// postgres
	query := `INSERT INTO sync_runs (run_id, src, dst, dry, total_scanned, total_bytes,
		copied, copied_bytes, skipped, extra, deleted, failed, started_at, finished_at, elapsed_ms, config_json)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
		ON CONFLICT (run_id) DO UPDATE SET
		src = EXCLUDED.src, dst = EXCLUDED.dst, dry = EXCLUDED.dry,
		total_scanned = EXCLUDED.total_scanned, total_bytes = EXCLUDED.total_bytes,
		copied = EXCLUDED.copied, copied_bytes = EXCLUDED.copied_bytes,
		skipped = EXCLUDED.skipped, extra = EXCLUDED.extra,
		deleted = EXCLUDED.deleted, failed = EXCLUDED.failed,
		started_at = EXCLUDED.started_at, finished_at = EXCLUDED.finished_at,
		elapsed_ms = EXCLUDED.elapsed_ms, config_json = EXCLUDED.config_json`
	args := []interface{}{s.RunID, s.Src, s.Dst, dry, s.TotalScanned, s.TotalBytes,
		s.Copied, s.CopiedBytes, s.Skipped, s.Extra, s.Deleted, s.Failed,
		s.StartedAt, s.FinishedAt, elapsed, s.ConfigJSON}
	return query, args
}

func (m *manager) migrateSummaryTable() error {
	exists, err := m.columnExistsInTable("sync_runs", "config_json")
	if err != nil || exists {
		return err
	}
	_, err = m.db.Exec("ALTER TABLE sync_runs ADD COLUMN config_json TEXT")
	return err
}

func (m *manager) columnExistsInTable(table, column string) (bool, error) {
	var query string
	if m.dbType == "mysql" {
		query = "SELECT 1 FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = ? AND column_name = ?"
	} else if m.dbType == "sqlite3" {
		query = "SELECT 1 FROM pragma_table_info(?) WHERE name = ?"
	} else {
		query = "SELECT 1 FROM information_schema.columns WHERE table_name = $1 AND column_name = $2"
	}
	var dummy int
	err := m.db.QueryRow(query, table, column).Scan(&dummy)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (m *manager) createSummaryTable() error {
	var pkClause string
	if m.dbType == "mysql" {
		pkClause = "PRIMARY KEY (run_id(64))"
	} else {
		pkClause = "PRIMARY KEY (run_id)"
	}
	createSQL := fmt.Sprintf(`CREATE TABLE IF NOT EXISTS sync_runs (
		run_id VARCHAR(64) NOT NULL,
		src VARCHAR(1024),
		dst VARCHAR(1024),
		dry INT,
		total_scanned BIGINT,
		total_bytes BIGINT,
		copied BIGINT,
		copied_bytes BIGINT,
		skipped BIGINT,
		extra BIGINT,
		deleted BIGINT,
		failed BIGINT,
		started_at DATETIME,
		finished_at DATETIME,
		elapsed_ms BIGINT,
		config_json TEXT,
		%s
	)`, pkClause)
	if m.dbType == "mysql" {
		createSQL += " ENGINE = InnoDB"
	}
	if _, err := m.db.Exec(createSQL); err != nil {
		return fmt.Errorf("create sync_runs table failed: %w", err)
	}
	return nil
}

func (m *manager) RecordSummary(s *RunSummary) error {
	if err := m.createSummaryTable(); err != nil {
		return err
	}
	_ = m.migrateSummaryTable()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	query, args := m.buildSummaryQueryAndArgs(s)
	_, err := m.db.ExecContext(ctx, query, args...)
	return err
}

func (m *manager) Close() error {
	return m.db.Close()
}

// sqlTask is a unified unit for batch execution.
type sqlTask struct {
	query string
	args  []interface{}
}

// batchRecorder provides asynchronous buffered recording with transaction batching.
type batchRecorder struct {
	m        *manager
	ch       chan *sqlTask
	done     chan struct{}
	wg       sync.WaitGroup
	mu       sync.Mutex
	buf      []*sqlTask
	interval time.Duration
}

func newBatchRecorder(m *manager, batchSize int, interval time.Duration) *batchRecorder {
	if batchSize <= 0 {
		batchSize = 100
	}
	if interval <= 0 {
		interval = time.Second
	}
	b := &batchRecorder{
		m:        m,
		ch:       make(chan *sqlTask, batchSize*2),
		done:     make(chan struct{}),
		interval: interval,
	}
	b.wg.Add(1)
	go b.loop(batchSize)
	return b
}

func (b *batchRecorder) submit(query string, args []interface{}) error {
	select {
	case b.ch <- &sqlTask{query: query, args: args}:
		return nil
	case <-time.After(100 * time.Millisecond):
		return b.m.exec(query, args...)
	}
}

func (b *batchRecorder) Record(r *SyncRecord) error {
	query, args := b.m.buildQueryAndArgs(r)
	return b.submit(query, args)
}

func (b *batchRecorder) RecordPlan(p *PlanRecord) error {
	query, args := b.m.buildPlanQueryAndArgs(p)
	return b.submit(query, args)
}

func (b *batchRecorder) RecordSummary(s *RunSummary) error {
	return b.m.RecordSummary(s)
}

func (b *batchRecorder) loop(batchSize int) {
	defer b.wg.Done()
	tick := time.NewTicker(b.interval)
	defer tick.Stop()

	flush := func() {
		b.mu.Lock()
		if len(b.buf) == 0 {
			b.mu.Unlock()
			return
		}
		items := make([]*sqlTask, len(b.buf))
		copy(items, b.buf)
		b.buf = b.buf[:0]
		b.mu.Unlock()

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		tx, err := b.m.db.BeginTx(ctx, nil)
		if err != nil {
			logger.Warnf("batch recorder begin tx: %s", err)
			for _, item := range items {
				if execErr := b.m.exec(item.query, item.args...); execErr != nil {
					logger.Warnf("batch recorder fallback exec: %s", execErr)
				}
			}
			return
		}
		defer tx.Rollback()

		for _, item := range items {
			if _, err := tx.ExecContext(ctx, item.query, item.args...); err != nil {
				logger.Warnf("batch recorder tx exec: %s", err)
			}
		}
		if err := tx.Commit(); err != nil {
			logger.Warnf("batch recorder commit: %s", err)
			for _, item := range items {
				if execErr := b.m.exec(item.query, item.args...); execErr != nil {
					logger.Warnf("batch recorder fallback exec: %s", execErr)
				}
			}
		}
	}

	for {
		select {
		case task := <-b.ch:
			b.mu.Lock()
			b.buf = append(b.buf, task)
			shouldFlush := len(b.buf) >= batchSize
			b.mu.Unlock()
			if shouldFlush {
				flush()
			}
		case <-tick.C:
			flush()
		case <-b.done:
			flush()
			return
		}
	}
}

func (b *batchRecorder) Close() error {
	close(b.done)
	b.wg.Wait()
	return b.m.Close()
}

type Callback struct {
	Recorder              Recorder
	ExtendedFieldsEnabled bool
	RunID                 string
}

func (c *Callback) OnTransferStart(sourceID, targetID string, isDir bool, size int64, mtime time.Time) {
	if c.Recorder == nil {
		return
	}
	r := &SyncRecord{
		SourceID:      sourceID,
		TargetID:      targetID,
		IsDirectory:   isDir,
		Size:          size,
		Mtime:         mtime,
		Status:        StatusInTransfer,
		TransferStart: time.Now(),
		RetryCount:    0,
	}
	if err := c.Recorder.Record(r); err != nil {
		logger.Warnf("record transfer start for %s: %s", sourceID, err)
	}
}

func (c *Callback) OnTransferComplete(sourceID, targetID string, size int64, targetMtime time.Time) {
	if c.Recorder == nil {
		return
	}
	r := &SyncRecord{
		SourceID:         sourceID,
		TargetID:         targetID,
		Size:             size,
		Status:           StatusTransferred,
		TransferComplete: time.Now(),
		TargetMtime:      targetMtime,
	}
	if err := c.Recorder.Record(r); err != nil {
		logger.Warnf("record transfer complete for %s: %s", sourceID, err)
	}
}

func (c *Callback) OnVerifyComplete(sourceID, targetID string, size int64, targetMtime time.Time) {
	if c.Recorder == nil {
		return
	}
	r := &SyncRecord{
		SourceID:       sourceID,
		TargetID:       targetID,
		Size:           size,
		Status:         StatusVerified,
		VerifyComplete: time.Now(),
		TargetMtime:    targetMtime,
	}
	if err := c.Recorder.Record(r); err != nil {
		logger.Warnf("record verify complete for %s: %s", sourceID, err)
	}
}

func (c *Callback) OnSkip(sourceID, targetID string, isDir bool, size int64, mtime time.Time) {
	if c.Recorder == nil {
		return
	}
	r := &SyncRecord{
		SourceID:    sourceID,
		TargetID:    targetID,
		IsDirectory: isDir,
		Size:        size,
		Mtime:       mtime,
		Status:      StatusSkipped,
	}
	if err := c.Recorder.Record(r); err != nil {
		logger.Warnf("record skip for %s: %s", sourceID, err)
	}
}

func (c *Callback) OnDelete(sourceID string, isDir bool) {
	if c.Recorder == nil {
		return
	}
	r := &SyncRecord{
		SourceID:        sourceID,
		IsDirectory:     isDir,
		Status:          StatusSourceDeleted,
		IsSourceDeleted: true,
	}
	if err := c.Recorder.Record(r); err != nil {
		logger.Warnf("record delete for %s: %s", sourceID, err)
	}
}

func (c *Callback) OnFailed(sourceID, targetID string, isDir bool, size int64, mtime time.Time, errMsg string) {
	if c.Recorder == nil {
		return
	}
	r := &SyncRecord{
		SourceID:          sourceID,
		TargetID:          targetID,
		IsDirectory:       isDir,
		Size:              size,
		Mtime:             mtime,
		Status:            StatusError,
		TransferComplete:  time.Now(),
		ErrorMessage:      errMsg,
		FirstErrorMessage: errMsg,
	}
	if err := c.Recorder.Record(r); err != nil {
		logger.Warnf("record failed for %s: %s", sourceID, err)
	}
}

func (c *Callback) OnRetry(sourceID string, retryCount int, errMsg string) {
	if c.Recorder == nil {
		return
	}
	r := &SyncRecord{
		SourceID:     sourceID,
		Status:       StatusInTransfer,
		RetryCount:   retryCount,
		ErrorMessage: errMsg,
	}
	if err := c.Recorder.Record(r); err != nil {
		logger.Warnf("record retry for %s: %s", sourceID, err)
	}
}

func (c *Callback) OnSummary(s *RunSummary) {
	if c.Recorder == nil {
		return
	}
	if err := c.Recorder.RecordSummary(s); err != nil {
		logger.Warnf("record summary: %s", err)
	}
}

func (c *Callback) OnPlanned(sourceID, targetID string, size int64, mtime time.Time, action, reason string) {
	if c.Recorder == nil {
		return
	}
	p := &PlanRecord{
		RunID:     c.RunID,
		SourceID:  sourceID,
		TargetID:  targetID,
		Size:      size,
		Mtime:     mtime,
		Action:    action,
		Reason:    reason,
		ScannedAt: time.Now(),
	}
	if err := c.Recorder.RecordPlan(p); err != nil {
		logger.Warnf("record plan for %s: %s", sourceID, err)
	}
}
