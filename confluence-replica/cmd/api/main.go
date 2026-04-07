package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"time"

	"confluence-replica/internal/app"
	"confluence-replica/internal/logx"
)

type server struct {
	rt *app.Runtime
}

type searchRequest struct {
	Query string `json:"query"`
	Limit int    `json:"limit"`
}

type chatRequest struct {
	Query string `json:"query"`
}

type jobRequest struct {
	ParentID string `json:"parent_id"`
	Date     string `json:"date"`
}

func main() {
	defaultConfigPath := os.Getenv("CONF_REPLICA_CONFIG")
	if defaultConfigPath == "" {
		defaultConfigPath = "config/config.yaml"
	}

	fs := flag.NewFlagSet("api", flag.ExitOnError)
	configPath := fs.String("config", defaultConfigPath, "path to config yaml")
	quiet := fs.Bool("quiet", false, "set log level to ERROR")
	verbose := fs.Bool("verbose", false, "set log level to DEBUG")
	_ = fs.Parse(os.Args[1:])

	cfg, err := app.LoadConfig(*configPath)
	if err != nil {
		log.Fatal(err)
	}
	if err := logx.Configure(cfg.Logging.Level, *quiet, *verbose); err != nil {
		log.Fatal(err)
	}
	rt, err := app.NewRuntime(context.Background(), cfg)
	if err != nil {
		log.Fatal(err)
	}
	defer rt.Close()

	s := &server{rt: rt}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /search", s.handleSearch)
	mux.HandleFunc("POST /chat", s.handleChat)
	mux.HandleFunc("GET /digest/", s.handleGetDigest)
	mux.HandleFunc("POST /jobs/bootstrap", s.handleJobBootstrap)
	mux.HandleFunc("POST /jobs/sync", s.handleJobSync)
	mux.HandleFunc("POST /jobs/digest", s.handleJobDigest)

	logx.Infof("[api] listening addr=%s", cfg.API.Addr)
	if err := http.ListenAndServe(cfg.API.Addr, mux); err != nil {
		log.Fatal(err)
	}
}

func (s *server) handleSearch(w http.ResponseWriter, r *http.Request) {
	var req searchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if req.Limit <= 0 || req.Limit > 50 {
		req.Limit = 10
	}
	hits, err := s.rt.Search.Query(r.Context(), req.Query, req.Limit)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"hits": hits})
}

func (s *server) handleChat(w http.ResponseWriter, r *http.Request) {
	var req chatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	resp, err := s.rt.RAG.Answer(r.Context(), req.Query)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *server) handleGetDigest(w http.ResponseWriter, r *http.Request) {
	dateText := r.PathValue("date")
	if dateText == "" {
		dateText = lastSegment(r.URL.Path)
	}
	day, err := time.Parse("2006-01-02", dateText)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	md, err := s.rt.Store.GetDigest(r.Context(), day)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"date": day.Format("2006-01-02"), "markdown": md})
}

func (s *server) handleJobBootstrap(w http.ResponseWriter, r *http.Request) {
	s.runSyncJob(w, r, true)
}

func (s *server) handleJobSync(w http.ResponseWriter, r *http.Request) {
	s.runSyncJob(w, r, false)
}

func (s *server) runSyncJob(w http.ResponseWriter, r *http.Request, bootstrap bool) {
	var req jobRequest
	_ = json.NewDecoder(r.Body).Decode(&req)
	if req.ParentID == "" {
		req.ParentID = s.rt.Config.Confluence.DefaultParentID
	}
	if req.ParentID == "" {
		writeErr(w, http.StatusBadRequest, errors.New("parent_id is required"))
		return
	}
	var err error
	if bootstrap {
		err = s.rt.Ingest.Bootstrap(r.Context(), req.ParentID)
	} else {
		err = s.rt.Ingest.Sync(r.Context(), req.ParentID)
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"status": "ok"})
}

func (s *server) handleJobDigest(w http.ResponseWriter, r *http.Request) {
	var req jobRequest
	_ = json.NewDecoder(r.Body).Decode(&req)
	day := time.Now()
	if req.Date != "" {
		d, err := time.Parse("2006-01-02", req.Date)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		day = d
	}
	md, err := s.rt.Digest.Generate(r.Context(), day)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"status": "ok", "markdown": md})
}

func writeErr(w http.ResponseWriter, code int, err error) {
	writeJSON(w, code, map[string]any{"error": err.Error()})
}

func writeJSON(w http.ResponseWriter, code int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(payload)
}

func lastSegment(p string) string {
	for len(p) > 0 && p[len(p)-1] == '/' {
		p = p[:len(p)-1]
	}
	for i := len(p) - 1; i >= 0; i-- {
		if p[i] == '/' {
			return p[i+1:]
		}
	}
	return p
}
