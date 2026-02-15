package api

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/passwordkeyorg/mail-ingest-service/internal/index"
	"github.com/passwordkeyorg/mail-ingest-service/internal/metrics"
	"github.com/passwordkeyorg/mail-ingest-service/internal/store"
)

type Deps struct {
	Logger   *slog.Logger
	DB       *index.DB
	Store    store.Store
	AdminKey string
	Metrics  *metrics.APIMetrics
}

type handler struct{ deps Deps }

func New(deps Deps) http.Handler {
	if deps.Logger == nil {
		deps.Logger = slog.Default()
	}
	mux := http.NewServeMux()
	h := &handler{deps: deps}

	mux.HandleFunc("GET /healthz", h.healthz)
	mux.HandleFunc("GET /v1/messages", h.list)
	mux.HandleFunc("GET /v1/messages/", h.getOrRaw)
	if deps.Metrics != nil {
		return instrument(*deps.Metrics, mux)
	}
	return mux
}

func (h *handler) requireAdmin(w http.ResponseWriter, r *http.Request) bool {
	if h.deps.AdminKey == "" {
		return true
	}
	got := r.Header.Get("X-Admin-Key")
	if got == h.deps.AdminKey {
		return true
	}
	w.WriteHeader(http.StatusUnauthorized)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": "unauthorized"})
	return false
}

func (h *handler) healthz(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func (h *handler) list(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}
	domain := r.URL.Query().Get("domain")
	mailbox := r.URL.Query().Get("mailbox")
	after := r.URL.Query().Get("after")
	if domain == "" || mailbox == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "domain and mailbox required"})
		return
	}
	rows, err := h.deps.DB.ListMessages(r.Context(), domain, mailbox, after, 50)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "db error"})
		return
	}
	next := ""
	if len(rows) > 0 {
		last := rows[len(rows)-1]
		next = last.ReceivedAt + ":" + last.ID
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"items": rows, "next": next})
}

func (h *handler) getOrRaw(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/v1/messages/")
	if path == "" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	id := path
	wantRaw := false
	if strings.HasSuffix(path, "/raw") {
		wantRaw = true
		id = strings.TrimSuffix(path, "/raw")
		id = strings.TrimSuffix(id, "/")
	}
	row, ok, err := h.deps.DB.GetMessage(r.Context(), id)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "db error"})
		return
	}
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "not found"})
		return
	}
	if !wantRaw {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(row)
		return
	}
	var rc io.ReadCloser
	if row.ObjectKey != "" {
		rc, err = h.deps.Store.OpenByObjectKey(r.Context(), row.ObjectKey)
	} else {
		rc, err = h.deps.Store.OpenByPath(row.EMLPath)
	}
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "open eml failed"})
		return
	}
	defer func() { _ = rc.Close() }()
	w.Header().Set("Content-Type", "message/rfc822")
	_, _ = io.Copy(w, rc)
}
