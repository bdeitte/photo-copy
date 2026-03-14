package mockserver

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

// GoogleMock is a configurable mock Google Photos HTTP server.
type GoogleMock struct {
	Server  *httptest.Server
	BaseURL string

	onUploadBytes HandlerFunc
	onBatchCreate HandlerFunc

	mu       sync.Mutex
	requests []RecordedRequest
}

// NewGoogle creates a new unconfigured Google Photos mock. Call builder methods then Start().
func NewGoogle(t *testing.T) *GoogleMock {
	m := &GoogleMock{
		onUploadBytes: defaultHandler("uploadBytes"),
		onBatchCreate: defaultHandler("batchCreate"),
	}
	t.Cleanup(func() {
		if m.Server != nil {
			m.Server.Close()
		}
	})
	return m
}

// OnUploadBytes sets the handler for POST /v1/uploads.
func (m *GoogleMock) OnUploadBytes(h HandlerFunc) *GoogleMock {
	m.onUploadBytes = h
	return m
}

// OnBatchCreate sets the handler for POST /v1/mediaItems:batchCreate.
func (m *GoogleMock) OnBatchCreate(h HandlerFunc) *GoogleMock {
	m.onBatchCreate = h
	return m
}

// Start creates and starts the httptest.Server. Returns m for chaining.
func (m *GoogleMock) Start() *GoogleMock {
	m.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recordRequest(r, &m.mu, &m.requests)

		switch r.URL.Path {
		case "/v1/uploads":
			m.onUploadBytes(w, r)
		case "/v1/mediaItems:batchCreate":
			m.onBatchCreate(w, r)
		default:
			http.Error(w, "unknown path: "+r.URL.Path, http.StatusNotFound)
		}
	}))
	m.BaseURL = m.Server.URL
	return m
}

// Requests returns a copy of all recorded requests.
func (m *GoogleMock) Requests() []RecordedRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]RecordedRequest, len(m.requests))
	copy(out, m.requests)
	return out
}
