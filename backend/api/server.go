// Package api exposes the project/proof HTTP API backed by Chi and SQLite.
package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"storyproof/store"
)

type Server struct {
	st *store.Store
	r  chi.Router

	// beforePersistProof, when set, runs after geometric evaluation and
	// before persisting a report. Tests use it to deterministically simulate
	// an edit racing the proof; it is never set in production.
	beforePersistProof func(projectID int64) error
}

func NewServer(st *store.Store) *Server {
	s := &Server{st: st}
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(corsMiddleware)
	r.Use(jsonContentType)

	r.Get("/api/health", s.health)

	r.Route("/api/projects", func(r chi.Router) {
		r.Post("/", s.createProject)
		r.Get("/", s.listProjects)
		r.Get("/{id}", s.getProject)
		r.Patch("/{id}", s.renameProject)
		r.Delete("/{id}", s.deleteProject)
		r.Post("/{id}/unfreeze", s.unfreezeProject)
		r.Post("/{id}/proofs", s.runProof)
		r.Get("/{id}/reports", s.listReports)

		r.Route("/{id}/sheets", func(r chi.Router) {
			r.Post("/", s.createSheet)
			r.Get("/", s.listSheets)
		})
		r.Route("/{id}/acts", func(r chi.Router) {
			r.Post("/", s.createAct)
			r.Get("/", s.listActs)
		})
	})

	// Sheet/act ids are globally unique; keep top-level routes for updates.
	r.Route("/api/sheets/{id}", func(r chi.Router) {
		r.Get("/", s.getSheet)
		r.Put("/", s.updateSheet)
		r.Delete("/", s.deleteSheet)
	})
	r.Route("/api/acts/{id}", func(r chi.Router) {
		r.Get("/", s.getAct)
		r.Put("/", s.updateAct)
		r.Delete("/", s.deleteAct)
	})
	r.Route("/api/reports/{id}", func(r chi.Router) {
		r.Get("/", s.getReport)
		r.Post("/confirm", s.confirmReport)
	})

	s.r = r
	return s
}

func (s *Server) Handler() http.Handler { return s.r }

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Methods", "GET,POST,PUT,PATCH,DELETE,OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func jsonContentType(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		next.ServeHTTP(w, r)
	})
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

type errBody struct {
	Error string `json:"error"`
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, errBody{Error: msg})
}

// mapStoreError translates storage errors into HTTP statuses.
func mapStoreError(w http.ResponseWriter, err error) bool {
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeErr(w, http.StatusNotFound, "not found")
	case errors.Is(err, store.ErrFrozen):
		writeErr(w, http.StatusConflict, "project inputs are frozen; unfreeze the confirmed report before editing")
	case errors.Is(err, store.ErrStaleReport):
		writeErr(w, http.StatusConflict, err.Error())
	default:
		return false
	}
	return true
}

func decode(w http.ResponseWriter, r *http.Request, v any) error {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return err
	}
	return nil
}
