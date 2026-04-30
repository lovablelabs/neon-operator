package fakes

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"

	"oltp.molnett.org/neon-operator/utils"
)

// Compute is a fake of compute_ctl's HTTP API.
type Compute struct {
	server     *httptest.Server
	jwtManager *utils.JWTManager

	// Configure overrides the default 200 handler for POST /configure when non-nil.
	Configure http.HandlerFunc

	mu    sync.Mutex
	calls []ConfigureCall
}

// ConfigureCall records a /configure request the fake received.
type ConfigureCall struct {
	Body   []byte
	Token  string
	Claims map[string]any
	// VerifyErr is set when JWT verification was attempted and failed.
	VerifyErr error
}

// NewCompute starts a fake compute_ctl HTTP server. If jm is non-nil, the fake
// verifies the Authorization bearer JWT against jm's public key and parses
// claims into ConfigureCall.Claims. If jm is nil, the raw token is captured
// without verification.
func NewCompute(jm *utils.JWTManager) *Compute {
	c := &Compute{jwtManager: jm}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /configure", c.handleConfigure)
	c.server = httptest.NewServer(mux)
	return c
}

func (c *Compute) URL() string { return c.server.URL }
func (c *Compute) Close()      { c.server.Close() }

// Configures returns a copy of every /configure request recorded.
func (c *Compute) Configures() []ConfigureCall {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]ConfigureCall, len(c.calls))
	copy(out, c.calls)
	return out
}

func (c *Compute) Reset() {
	c.mu.Lock()
	c.calls = nil
	c.mu.Unlock()
}

func (c *Compute) handleConfigure(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	_ = r.Body.Close()

	call := ConfigureCall{Body: body}
	if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
		call.Token = strings.TrimPrefix(auth, "Bearer ")
	}

	if c.jwtManager != nil && call.Token != "" {
		token, err := c.jwtManager.VerifyToken(call.Token)
		if err != nil {
			call.VerifyErr = err
		} else {
			call.Claims = make(map[string]any)
			for _, k := range token.Keys() {
				var v any
				if err := token.Get(k, &v); err == nil {
					call.Claims[k] = v
				}
			}
		}
	}

	c.mu.Lock()
	c.calls = append(c.calls, call)
	c.mu.Unlock()

	if c.Configure != nil {
		r.Body = io.NopCloser(bytes.NewReader(body))
		c.Configure(w, r)
		return
	}
	w.WriteHeader(http.StatusOK)
}
