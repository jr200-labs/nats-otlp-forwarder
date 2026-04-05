//go:build integration

package integration

import (
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

// ReceivedRequest captures what the fake collector received.
type ReceivedRequest struct {
	Path        string
	ContentType string
	Body        []byte
}

// FakeCollector is an httptest.Server that records received OTLP requests and
// can be told to return error codes for specific signals.
type FakeCollector struct {
	Server   *httptest.Server
	Received chan ReceivedRequest

	mu        sync.Mutex
	responder func(path string, callCount int) int // returns HTTP status code
	callCount map[string]int
}

// NewFakeCollector starts a fake collector that returns 200 for all requests by default.
func NewFakeCollector(t *testing.T) *FakeCollector {
	t.Helper()
	fc := &FakeCollector{
		Received:  make(chan ReceivedRequest, 64),
		callCount: map[string]int{},
		responder: func(string, int) int { return http.StatusOK },
	}
	fc.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		fc.mu.Lock()
		fc.callCount[r.URL.Path]++
		code := fc.responder(r.URL.Path, fc.callCount[r.URL.Path])
		fc.mu.Unlock()

		fc.Received <- ReceivedRequest{
			Path:        r.URL.Path,
			ContentType: r.Header.Get("Content-Type"),
			Body:        body,
		}
		w.WriteHeader(code)
	}))
	t.Cleanup(fc.Server.Close)
	return fc
}

// SetResponder installs a custom response function: (path, call-count-for-that-path) -> status code.
func (fc *FakeCollector) SetResponder(fn func(path string, callCount int) int) {
	fc.mu.Lock()
	defer fc.mu.Unlock()
	fc.responder = fn
}

// CallCount returns the number of times the given path has been hit.
func (fc *FakeCollector) CallCount(path string) int {
	fc.mu.Lock()
	defer fc.mu.Unlock()
	return fc.callCount[path]
}
