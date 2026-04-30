// Package fakes provides in-memory HTTP test doubles for the upstream services
// the operator talks to: storage_controller and compute_ctl. Use them to point
// reconcilers and controlplane handlers at a reachable server during tests.
package fakes

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
)

// StorageController is a fake of the upstream storage_controller HTTP API.
type StorageController struct {
	server *httptest.Server

	// LocationConfig overrides the default 200 handler for
	// PUT /v1/tenant/{id}/location_config when non-nil.
	LocationConfig http.HandlerFunc

	// Timeline overrides the default 201 handler for
	// POST /v1/tenant/{id}/timeline when non-nil.
	Timeline http.HandlerFunc

	mu    sync.Mutex
	calls []Call
}

// Call records a request the fake received.
type Call struct {
	Method string
	Path   string
	Body   []byte
}

// NewStorageController starts a fake storage_controller HTTP server.
// Callers must Close() it when done.
func NewStorageController() *StorageController {
	sc := &StorageController{}
	mux := http.NewServeMux()
	mux.HandleFunc("PUT /v1/tenant/{id}/location_config", func(w http.ResponseWriter, r *http.Request) {
		sc.dispatch(w, r, sc.LocationConfig, http.StatusOK)
	})
	mux.HandleFunc("POST /v1/tenant/{id}/timeline", func(w http.ResponseWriter, r *http.Request) {
		sc.dispatch(w, r, sc.Timeline, http.StatusCreated)
	})
	sc.server = httptest.NewServer(mux)
	return sc
}

// URL returns the base URL of the fake server, suitable for
// ProjectReconciler.StorageControllerBaseURL or equivalent.
func (sc *StorageController) URL() string { return sc.server.URL }

// Close shuts the fake down. Safe to call multiple times.
func (sc *StorageController) Close() { sc.server.Close() }

// Calls returns a copy of every recorded request, in arrival order.
func (sc *StorageController) Calls() []Call {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	out := make([]Call, len(sc.calls))
	copy(out, sc.calls)
	return out
}

// Reset clears the recorded calls. Useful for re-using a fake across subtests.
func (sc *StorageController) Reset() {
	sc.mu.Lock()
	sc.calls = nil
	sc.mu.Unlock()
}

func (sc *StorageController) dispatch(
	w http.ResponseWriter,
	r *http.Request,
	override http.HandlerFunc,
	defaultStatus int,
) {
	body, _ := io.ReadAll(r.Body)
	_ = r.Body.Close()

	sc.mu.Lock()
	sc.calls = append(sc.calls, Call{Method: r.Method, Path: r.URL.Path, Body: body})
	sc.mu.Unlock()

	if override != nil {
		r.Body = io.NopCloser(bytes.NewReader(body))
		override(w, r)
		return
	}
	w.WriteHeader(defaultStatus)
}
