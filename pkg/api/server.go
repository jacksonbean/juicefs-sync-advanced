package api

import (
	"database/sql"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

type Server struct {
	port        int
	mux         *http.ServeMux
	handler     *APIHandler
	historyDB   *sql.DB
	scheduleDB  *sql.DB
	scheduler   *Scheduler
	staticFS    fs.FS
	historyPath string
	schedPath   string
}

func NewServer(port int, historyDBPath, schedDBPath string, staticFS fs.FS) *Server {
	s := &Server{
		port:        port,
		mux:         http.NewServeMux(),
		staticFS:    staticFS,
		historyPath: historyDBPath,
		schedPath:   schedDBPath,
	}
	s.setupRoutes()
	return s
}

func (s *Server) setupRoutes() {
	wrap := func(h http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			if r.Method == "OPTIONS" {
				w.WriteHeader(204)
				return
			}
			h(w, r)
		}
	}

	s.mux.HandleFunc("/api/instances", wrap(s.handler.HandleInstances))
	s.mux.HandleFunc("/api/metrics", wrap(s.handler.HandleMetricsProxy))
	s.mux.HandleFunc("/api/metrics/aggregated", wrap(s.handler.HandleAggregatedMetrics))
	s.mux.HandleFunc("/api/history", wrap(s.handler.HandleHistory))
	s.mux.HandleFunc("/api/history/", wrap(s.handler.HandleHistory))
	s.mux.HandleFunc("/api/failed", wrap(s.handler.HandleFailed))
	s.mux.HandleFunc("/api/templates", wrap(s.handler.HandleTemplates))
	s.mux.HandleFunc("/api/templates/", wrap(s.handler.HandleTemplates))
	s.mux.HandleFunc("/api/schedules", wrap(s.handler.HandleSchedules))
	s.mux.HandleFunc("/api/schedules/", wrap(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/history") {
			s.handler.HandleScheduleHistory(w, r)
			return
		}
		s.handler.HandleSchedules(w, r)
	}))
	s.mux.HandleFunc("/api/sync", wrap(s.handler.HandleSync))
	s.mux.HandleFunc("/api/health", wrap(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`))
	}))

	if s.staticFS != nil {
		s.mux.Handle("/", http.FileServer(http.FS(s.staticFS)))
	}
}

func (s *Server) openDatabases() error {
	dir := filepath.Dir(s.historyPath)
	os.MkdirAll(dir, 0755)

	var err error
	s.historyDB, err = openDB(s.historyPath)
	if err != nil {
		return fmt.Errorf("open history db: %w", err)
	}
	s.scheduleDB, err = openDB(s.schedPath)
	if err != nil {
		return fmt.Errorf("open schedule db: %w", err)
	}

	s.handler = NewAPIHandler(s.historyDB, s.scheduleDB)
	return nil
}

func (s *Server) StartScheduler() {
	s.scheduler = NewScheduler(s.schedPath, s.historyPath)
	go func() {
		if err := s.scheduler.Run(); err != nil {
			log.Printf("scheduler error: %v", err)
		}
	}()
}

func (s *Server) ListenAndServe() error {
	if err := s.openDatabases(); err != nil {
		return fmt.Errorf("open databases: %w", err)
	}

	if s.scheduler != nil {
		defer s.scheduler.Stop()
	}
	if s.historyDB != nil {
		defer s.historyDB.Close()
	}
	if s.scheduleDB != nil {
		defer s.scheduleDB.Close()
	}

	addr := fmt.Sprintf("127.0.0.1:%d", s.port)
	log.Printf("API server listening on http://%s", addr)
	log.Printf("Open http://%s in your browser", addr)
	s.setupRoutes()
	return http.ListenAndServe(addr, s.mux)
}
