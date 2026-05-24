package api

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type Scheduler struct {
	dbPath    string
	historyDB string
	db        *sql.DB
	stopCh    chan struct{}
	wg        sync.WaitGroup
}

func NewScheduler(dbPath, historyDB string) *Scheduler {
	return &Scheduler{
		dbPath:    dbPath,
		historyDB: historyDB,
		stopCh:    make(chan struct{}),
	}
}

func (s *Scheduler) Run() error {
	var err error
	s.db, err = sql.Open("sqlite3", s.dbPath+"?_journal_mode=WAL")
	if err != nil {
		return fmt.Errorf("open scheduler db: %w", err)
	}
	defer s.db.Close()

	if err := s.initDB(); err != nil {
		return fmt.Errorf("init scheduler db: %w", err)
	}

	log.Printf("Background scheduler started")
	s.wg.Add(1)
	defer s.wg.Done()

	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.checkTasks()
		case <-s.stopCh:
			return nil
		}
	}
}

func (s *Scheduler) Stop() {
	close(s.stopCh)
	s.wg.Wait()
}

func (s *Scheduler) initDB() error {
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS scheduled_tasks (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		src TEXT NOT NULL,
		dst TEXT NOT NULL,
		cron_expr TEXT NOT NULL,
		enabled INTEGER DEFAULT 1,
		threads INTEGER DEFAULT 10,
		options TEXT DEFAULT '{}',
		last_run TEXT,
		next_run TEXT,
		created_at TEXT DEFAULT CURRENT_TIMESTAMP
	)`)
	if err != nil { return err }
	_, err = s.db.Exec(`CREATE TABLE IF NOT EXISTS schedule_history (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		task_id INTEGER,
		started_at TEXT,
		finished_at TEXT,
		status TEXT,
		output TEXT,
		error TEXT,
		objects_copied INTEGER DEFAULT 0
	)`)
	return err
}

func (s *Scheduler) checkTasks() {
	rows, err := s.db.Query(`SELECT id, name, src, dst, cron_expr, threads, options FROM scheduled_tasks WHERE enabled = 1`)
	if err != nil { return }
	defer rows.Close()

	for rows.Next() {
		var id int; var name, src, dst, cronExpr, options string; var threads int
		if err := rows.Scan(&id, &name, &src, &dst, &cronExpr, &threads, &options); err != nil { continue }
		if s.shouldRun(cronExpr) {
			go s.executeTask(id, name, src, dst, threads, options)
		}
	}
}

func (s *Scheduler) shouldRun(cronExpr string) bool {
	parts := strings.Fields(cronExpr)
	if len(parts) != 5 { return false }
	now := time.Now()
	return matchField(parts[0], now.Minute()) &&
		matchField(parts[1], now.Hour()) &&
		matchField(parts[2], now.Day()) &&
		matchField(parts[3], int(now.Month())) &&
		matchField(parts[4], int(now.Weekday()))
}

func matchField(field string, val int) bool {
	if field == "*" { return true }
	for _, part := range strings.Split(field, ",") {
		if strings.Contains(part, "/") {
			parts := strings.SplitN(part, "/", 2)
			step, _ := strconv.Atoi(parts[1])
			if step == 0 { continue }
			if parts[0] == "*" {
				if val%step == 0 { return true }
			} else {
				start, _ := strconv.Atoi(parts[0])
				if val >= start && (val-start)%step == 0 { return true }
			}
		} else if strings.Contains(part, "-") {
			parts := strings.SplitN(part, "-", 2)
			lo, _ := strconv.Atoi(parts[0])
			hi, _ := strconv.Atoi(parts[1])
			if val >= lo && val <= hi { return true }
		} else {
			v, _ := strconv.Atoi(part)
			if v == val { return true }
		}
	}
	return false
}

func (s *Scheduler) executeTask(id int, name, src, dst string, threads int, options string) {
	startTime := time.Now()
	s.db.Exec(`UPDATE scheduled_tasks SET last_run = ? WHERE id = ?`, startTime.Format(time.RFC3339), id)

	binary, _ := os.Executable()
	if binary == "" { binary = "juicefs-sync-advanced" }
	args := []string{binary, "sync", src, dst, "-p", strconv.Itoa(threads)}
	if s.historyDB != "" {
		args = append(args, "--record-db-type", "sqlite3", "--record-db-dsn", s.historyDB, "--instance-name", "[scheduled] "+name)
	}

	cmd := exec.Command(args[0], args[1:]...)
	output, err := cmd.CombinedOutput()
	finishedAt := time.Now()
	status := "success"
	errMsg := ""
	if err != nil {
		status = "failed"
		errMsg = err.Error()
	}
	s.db.Exec(`INSERT INTO schedule_history (task_id, started_at, finished_at, status, output, error)
		VALUES (?, ?, ?, ?, ?, ?)`, id, startTime.Format(time.RFC3339), finishedAt.Format(time.RFC3339),
		status, string(output), errMsg)
}
