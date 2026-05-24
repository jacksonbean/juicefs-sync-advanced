package api

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type InstanceInfo struct {
	PID       int    `json:"pid"`
	Port      int    `json:"port"`
	Metrics   string `json:"metrics"`
	Src       string `json:"src"`
	Dst       string `json:"dst"`
	Name      string `json:"name"`
	StartTime string `json:"start_time"`
}

type RunSummary struct {
	RunID        string  `json:"run_id"`
	Src          string  `json:"src"`
	Dst          string  `json:"dst"`
	Dry          bool    `json:"dry"`
	TotalScanned int64   `json:"total_scanned"`
	TotalBytes   int64   `json:"total_bytes"`
	Copied       int64   `json:"copied"`
	CopiedBytes  int64   `json:"copied_bytes"`
	Skipped      int64   `json:"skipped"`
	Extra        int64   `json:"extra"`
	Deleted      int64   `json:"deleted"`
	Failed       int64   `json:"failed"`
	StartedAt    string  `json:"started_at"`
	FinishedAt   string  `json:"finished_at"`
	ElapsedMs    int64   `json:"elapsed_ms"`
	ProgressPct  float64 `json:"progress_pct"`
	ConfigJSON   string  `json:"config_json,omitempty"`
}

type RunError struct {
	SourceID     string `json:"source_id"`
	TargetID     string `json:"target_id"`
	Size         int64  `json:"size"`
	ErrorMessage string `json:"error_message"`
	AttemptedAt  string `json:"attempted_at"`
}

type RunDetail struct {
	Run    RunSummary `json:"run"`
	Errors []RunError `json:"errors"`
}

type FailedObject struct {
	SourceID     string `json:"source_id"`
	TargetID     string `json:"target_id"`
	Size         int64  `json:"size"`
	Status       string `json:"status"`
	ErrorMessage string `json:"error_message"`
	AttemptedAt  string `json:"attempted_at"`
	RetryCount   int    `json:"retry_count"`
}

type Template struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	SrcTemplate string `json:"src_template"`
	DstTemplate string `json:"dst_template"`
	Threads     int    `json:"threads"`
	Options     string `json:"options"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

type ScheduleTask struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	Src       string `json:"src"`
	Dst       string `json:"dst"`
	CronExpr  string `json:"cron_expr"`
	Enabled   bool   `json:"enabled"`
	Threads   int    `json:"threads"`
	Options   string `json:"options"`
	LastRun   string `json:"last_run"`
	NextRun   string `json:"next_run"`
	CreatedAt string `json:"created_at"`
}

type ScheduleHistory struct {
	ID            int    `json:"id"`
	TaskID        int    `json:"task_id"`
	StartedAt     string `json:"started_at"`
	FinishedAt    string `json:"finished_at"`
	Status        string `json:"status"`
	Output        string `json:"output"`
	Error         string `json:"error"`
	ObjectsCopied int    `json:"objects_copied"`
}

type SyncRequest struct {
	Src       string `json:"src"`
	Dst       string `json:"dst"`
	Threads   int    `json:"threads"`
	DryRun    bool   `json:"dry_run"`
	DeleteDst bool   `json:"delete_dst"`
	Update    bool   `json:"update"`
}

type APIHandler struct {
	historyDB  *sql.DB
	scheduleDB *sql.DB
}

func NewAPIHandler(historyDB, scheduleDB *sql.DB) *APIHandler {
	return &APIHandler{
		historyDB:  historyDB,
		scheduleDB: scheduleDB,
	}
}

func jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func jsonOK(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func parseID(r *http.Request) (int, error) {
	parts := strings.Split(r.URL.Path, "/")
	for i := len(parts) - 1; i >= 0; i-- {
		if id, err := strconv.Atoi(parts[i]); err == nil {
			return id, nil
		}
	}
	return 0, fmt.Errorf("id not found")
}

func (h *APIHandler) HandleInstances(w http.ResponseWriter, r *http.Request) {
	data, err := os.ReadFile(DefaultInstanceRegistryPath())
	if err != nil {
		jsonOK(w, []InstanceInfo{})
		return
	}
	var instances []InstanceInfo
	if err := json.Unmarshal(data, &instances); err != nil {
		jsonOK(w, []InstanceInfo{})
		return
	}
	alive := make([]InstanceInfo, 0, len(instances))
	for _, inst := range instances {
		if inst.PID == 0 {
			alive = append(alive, inst)
			continue
		}
		p, err := os.FindProcess(inst.PID)
		if err == nil && p.Signal(syscall.Signal(0)) == nil {
			alive = append(alive, inst)
		}
	}
	jsonOK(w, alive)
}

func (h *APIHandler) HandleMetricsProxy(w http.ResponseWriter, r *http.Request) {
	portStr := r.URL.Query().Get("port")
	if portStr == "" {
		jsonError(w, "port parameter required", http.StatusBadRequest)
		return
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		jsonError(w, "invalid port", http.StatusBadRequest)
		return
	}
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/metrics", port))
	if err != nil {
		jsonError(w, "failed to fetch metrics: "+err.Error(), http.StatusGatewayTimeout)
		return
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		jsonError(w, "failed to read metrics", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	w.Write(body)
}

func (h *APIHandler) HandleAggregatedMetrics(w http.ResponseWriter, r *http.Request) {
	data, err := os.ReadFile(DefaultInstanceRegistryPath())
	if err != nil {
		jsonOK(w, map[string]int{"total": 0, "running": 0, "scanned": 0, "copied": 0, "failed": 0})
		return
	}
	var instances []InstanceInfo
	if err := json.Unmarshal(data, &instances); err != nil {
		jsonOK(w, map[string]int{"total": 0, "running": 0, "scanned": 0, "copied": 0, "failed": 0})
		return
	}
	total, running := 0, 0
	for _, inst := range instances {
		total++
		p, err := os.FindProcess(inst.PID)
		if err == nil && p.Signal(syscall.Signal(0)) == nil {
			running++
		}
	}
	jsonOK(w, map[string]int{"total": total, "running": running, "scanned": 0, "copied": 0, "failed": 0})
}

func (h *APIHandler) HandleHistory(w http.ResponseWriter, r *http.Request) {
	if h.historyDB == nil {
		jsonOK(w, []RunSummary{})
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/history")
	path = strings.Trim(path, "/")
	if path != "" {
		parts := strings.Split(path, "/")
		runID := parts[0]
		isCSV := len(parts) > 1 && parts[1] == "errors.csv"
		h.handleRunDetail(w, runID, isCSV)
		return
	}

	rows, err := h.historyDB.Query(`
		SELECT run_id, src, dst, dry, total_scanned, total_bytes,
			copied, copied_bytes, skipped, extra, deleted, failed,
			COALESCE(started_at,''), COALESCE(finished_at,''), COALESCE(elapsed_ms,0),
			COALESCE(config_json,'')
		FROM sync_runs ORDER BY started_at DESC LIMIT 100
	`)
	if err != nil {
		jsonOK(w, []RunSummary{})
		return
	}
	defer rows.Close()

	summaries := make([]RunSummary, 0)
	for rows.Next() {
		var s RunSummary
		var dry int
		if err := rows.Scan(&s.RunID, &s.Src, &s.Dst, &dry,
			&s.TotalScanned, &s.TotalBytes, &s.Copied, &s.CopiedBytes,
			&s.Skipped, &s.Extra, &s.Deleted, &s.Failed,
			&s.StartedAt, &s.FinishedAt, &s.ElapsedMs, &s.ConfigJSON); err != nil {
			continue
		}
		s.Dry = dry != 0
		if s.TotalScanned > 0 {
			s.ProgressPct = float64(s.Copied) / float64(s.TotalScanned) * 100
		}
		summaries = append(summaries, s)
	}
	jsonOK(w, summaries)
}

func (h *APIHandler) handleRunDetail(w http.ResponseWriter, runID string, asCSV bool) {
	var s RunSummary
	var dry int
	err := h.historyDB.QueryRow(`
		SELECT run_id, src, dst, dry, total_scanned, total_bytes,
			copied, copied_bytes, skipped, extra, deleted, failed,
			COALESCE(started_at,''), COALESCE(finished_at,''), COALESCE(elapsed_ms,0),
			COALESCE(config_json,'')
		FROM sync_runs WHERE run_id = ?
	`, runID).Scan(&s.RunID, &s.Src, &s.Dst, &dry,
		&s.TotalScanned, &s.TotalBytes, &s.Copied, &s.CopiedBytes,
		&s.Skipped, &s.Extra, &s.Deleted, &s.Failed,
		&s.StartedAt, &s.FinishedAt, &s.ElapsedMs, &s.ConfigJSON)
	if err != nil {
		jsonError(w, "run not found", http.StatusNotFound)
		return
	}
	s.Dry = dry != 0
	if s.TotalScanned > 0 {
		s.ProgressPct = float64(s.Copied) / float64(s.TotalScanned) * 100
	}

	rows, err := h.historyDB.Query(`
		SELECT source_id, COALESCE(target_id,''), COALESCE(size,0),
			COALESCE(error_message,''), COALESCE(transfer_complete, transfer_start, '')
		FROM objects
		WHERE status = 'Error'
		AND transfer_complete >= ? AND transfer_complete <= ?
		ORDER BY transfer_complete DESC LIMIT 500
	`, s.StartedAt, s.FinishedAt)
	if err != nil {
		jsonOK(w, RunDetail{Run: s, Errors: []RunError{}})
		return
	}
	defer rows.Close()

	var errors []RunError
	for rows.Next() {
		var e RunError
		if err := rows.Scan(&e.SourceID, &e.TargetID, &e.Size, &e.ErrorMessage, &e.AttemptedAt); err != nil {
			continue
		}
		errors = append(errors, e)
	}
	if errors == nil {
		errors = []RunError{}
	}

	if asCSV {
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"errors_%s.csv\"", runID))
		w.Write([]byte("source_id,target_id,size,error_message,attempted_at\n"))
		for _, e := range errors {
			w.Write([]byte(fmt.Sprintf("%s,%s,%d,%s,%s\n",
				escapeCSV(e.SourceID), escapeCSV(e.TargetID),
				e.Size, escapeCSV(e.ErrorMessage), escapeCSV(e.AttemptedAt))))
		}
		return
	}
	jsonOK(w, RunDetail{Run: s, Errors: errors})
}

func escapeCSV(s string) string {
	if strings.ContainsAny(s, ",\"\n") {
		return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
	}
	return s
}

func (h *APIHandler) HandleFailed(w http.ResponseWriter, r *http.Request) {
	if h.historyDB == nil {
		jsonOK(w, []FailedObject{})
		return
	}
	rows, err := h.historyDB.Query(`
		SELECT source_id, COALESCE(target_id,''), COALESCE(size,0),
			status, COALESCE(error_message,''),
			COALESCE(transfer_complete, transfer_start, ''),
			COALESCE(retry_count,0)
		FROM objects WHERE status = 'Error'
		ORDER BY transfer_complete DESC LIMIT 100
	`)
	if err != nil {
		jsonOK(w, []FailedObject{})
		return
	}
	defer rows.Close()
	objects := make([]FailedObject, 0)
	for rows.Next() {
		var o FailedObject
		if err := rows.Scan(&o.SourceID, &o.TargetID, &o.Size,
			&o.Status, &o.ErrorMessage, &o.AttemptedAt, &o.RetryCount); err != nil {
			continue
		}
		objects = append(objects, o)
	}
	jsonOK(w, objects)
}

func (h *APIHandler) HandleTemplates(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.listTemplates(w, r)
	case http.MethodPost:
		h.createTemplate(w, r)
	case http.MethodPut:
		h.updateTemplate(w, r)
	case http.MethodDelete:
		h.deleteTemplate(w, r)
	default:
		jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *APIHandler) listTemplates(w http.ResponseWriter, r *http.Request) {
	if h.scheduleDB == nil { jsonOK(w, []Template{}); return }
	rows, err := h.scheduleDB.Query(`SELECT id, name, COALESCE(description,''),
		src_template, dst_template, threads, COALESCE(options,'{}'),
		COALESCE(created_at,''), COALESCE(updated_at,'') FROM templates ORDER BY name`)
	if err != nil { jsonOK(w, []Template{}); return }
	defer rows.Close()
	templates := make([]Template, 0)
	for rows.Next() {
		var t Template
		if err := rows.Scan(&t.ID, &t.Name, &t.Description, &t.SrcTemplate, &t.DstTemplate,
			&t.Threads, &t.Options, &t.CreatedAt, &t.UpdatedAt); err != nil { continue }
		templates = append(templates, t)
	}
	jsonOK(w, templates)
}

func (h *APIHandler) createTemplate(w http.ResponseWriter, r *http.Request) {
	if h.scheduleDB == nil { jsonError(w, "db not available", 500); return }
	var t Template
	if err := json.NewDecoder(r.Body).Decode(&t); err != nil { jsonError(w, "bad request", 400); return }
	_, err := h.scheduleDB.Exec(`INSERT INTO templates (name, description, src_template, dst_template, threads, options) VALUES (?,?,?,?,?,?)`,
		t.Name, t.Description, t.SrcTemplate, t.DstTemplate, t.Threads, t.Options)
	if err != nil { jsonError(w, err.Error(), 500); return }
	jsonOK(w, map[string]string{"status": "ok"})
}

func (h *APIHandler) updateTemplate(w http.ResponseWriter, r *http.Request) {
	if h.scheduleDB == nil { jsonError(w, "db not available", 500); return }
	id, err := parseID(r); if err != nil { jsonError(w, "invalid id", 400); return }
	var t Template
	if err := json.NewDecoder(r.Body).Decode(&t); err != nil { jsonError(w, "bad request", 400); return }
	_, err = h.scheduleDB.Exec(`UPDATE templates SET name=?, description=?, src_template=?, dst_template=?, threads=?, options=? WHERE id=?`,
		t.Name, t.Description, t.SrcTemplate, t.DstTemplate, t.Threads, t.Options, id)
	if err != nil { jsonError(w, err.Error(), 500); return }
	jsonOK(w, map[string]string{"status": "ok"})
}

func (h *APIHandler) deleteTemplate(w http.ResponseWriter, r *http.Request) {
	if h.scheduleDB == nil { jsonError(w, "db not available", 500); return }
	id, err := parseID(r); if err != nil { jsonError(w, "invalid id", 400); return }
	h.scheduleDB.Exec("DELETE FROM templates WHERE id=?", id)
	jsonOK(w, map[string]string{"status": "ok"})
}

func (h *APIHandler) HandleSchedules(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet: h.listSchedules(w, r)
	case http.MethodPost: h.createSchedule(w, r)
	case http.MethodPut: h.updateSchedule(w, r)
	case http.MethodDelete: h.deleteSchedule(w, r)
	default: jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *APIHandler) listSchedules(w http.ResponseWriter, r *http.Request) {
	if h.scheduleDB == nil { jsonOK(w, []ScheduleTask{}); return }
	rows, err := h.scheduleDB.Query(`SELECT id, name, src, dst, cron_expr, enabled, threads,
		COALESCE(options,'{}'), COALESCE(last_run,''), COALESCE(next_run,''), COALESCE(created_at,'')
		FROM scheduled_tasks ORDER BY name`)
	if err != nil { jsonOK(w, []ScheduleTask{}); return }
	defer rows.Close()
	tasks := make([]ScheduleTask, 0)
	for rows.Next() {
		var t ScheduleTask
		if err := rows.Scan(&t.ID, &t.Name, &t.Src, &t.Dst, &t.CronExpr, &t.Enabled,
			&t.Threads, &t.Options, &t.LastRun, &t.NextRun, &t.CreatedAt); err != nil { continue }
		tasks = append(tasks, t)
	}
	jsonOK(w, tasks)
}

func (h *APIHandler) createSchedule(w http.ResponseWriter, r *http.Request) {
	if h.scheduleDB == nil { jsonError(w, "db not available", 500); return }
	var t ScheduleTask
	if err := json.NewDecoder(r.Body).Decode(&t); err != nil { jsonError(w, "bad request", 400); return }
	_, err := h.scheduleDB.Exec(`INSERT INTO scheduled_tasks (name, src, dst, cron_expr, enabled, threads, options) VALUES (?,?,?,?,?,?,?)`,
		t.Name, t.Src, t.Dst, t.CronExpr, t.Enabled, t.Threads, t.Options)
	if err != nil { jsonError(w, err.Error(), 500); return }
	jsonOK(w, map[string]string{"status": "ok"})
}

func (h *APIHandler) updateSchedule(w http.ResponseWriter, r *http.Request) {
	if h.scheduleDB == nil { jsonError(w, "db not available", 500); return }
	id, err := parseID(r); if err != nil { jsonError(w, "invalid id", 400); return }
	var t ScheduleTask
	if err := json.NewDecoder(r.Body).Decode(&t); err != nil { jsonError(w, "bad request", 400); return }
	_, err = h.scheduleDB.Exec(`UPDATE scheduled_tasks SET name=?, src=?, dst=?, cron_expr=?, enabled=?, threads=?, options=? WHERE id=?`,
		t.Name, t.Src, t.Dst, t.CronExpr, t.Enabled, t.Threads, t.Options, id)
	if err != nil { jsonError(w, err.Error(), 500); return }
	jsonOK(w, map[string]string{"status": "ok"})
}

func (h *APIHandler) deleteSchedule(w http.ResponseWriter, r *http.Request) {
	if h.scheduleDB == nil { jsonError(w, "db not available", 500); return }
	id, err := parseID(r); if err != nil { jsonError(w, "invalid id", 400); return }
	h.scheduleDB.Exec("DELETE FROM scheduled_tasks WHERE id=?", id)
	jsonOK(w, map[string]string{"status": "ok"})
}

func (h *APIHandler) HandleScheduleHistory(w http.ResponseWriter, r *http.Request) {
	if h.scheduleDB == nil { jsonOK(w, []ScheduleHistory{}); return }
	parts := strings.Split(r.URL.Path, "/")
	taskID := 0
	for _, p := range parts {
		if id, err := strconv.Atoi(p); err == nil { taskID = id; break }
	}
	if taskID == 0 { jsonError(w, "invalid task id", 400); return }
	rows, err := h.scheduleDB.Query(`SELECT id, task_id, COALESCE(started_at,''), COALESCE(finished_at,''),
		COALESCE(status,''), COALESCE(output,''), COALESCE(error,''), COALESCE(objects_copied,0)
		FROM schedule_history WHERE task_id=? ORDER BY started_at DESC LIMIT 50`, taskID)
	if err != nil { jsonOK(w, []ScheduleHistory{}); return }
	defer rows.Close()
	history := make([]ScheduleHistory, 0)
	for rows.Next() {
		var hh ScheduleHistory
		if err := rows.Scan(&hh.ID, &hh.TaskID, &hh.StartedAt, &hh.FinishedAt,
			&hh.Status, &hh.Output, &hh.Error, &hh.ObjectsCopied); err != nil { continue }
		history = append(history, hh)
	}
	jsonOK(w, history)
}

func (h *APIHandler) HandleSync(w http.ResponseWriter, r *http.Request) {
	var req SyncRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "bad request", 400)
		return
	}
	cmd := fmt.Sprintf("./juicefs-sync-advanced sync %s %s -p %d", req.Src, req.Dst, req.Threads)
	if req.DryRun { cmd += " --dry" }
	if req.DeleteDst { cmd += " --delete-dst" }
	if req.Update { cmd += " --update" }
	jsonOK(w, map[string]string{"command": cmd, "message": "执行以下命令启动同步"})
}
