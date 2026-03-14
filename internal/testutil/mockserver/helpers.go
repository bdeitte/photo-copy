// Package mockserver provides configurable mock HTTP servers for integration testing.
package mockserver

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"sync"
)

// RecordedRequest captures request details for test assertions.
type RecordedRequest struct {
	Method  string
	Path    string
	Query   url.Values
	Headers http.Header
	Body    []byte
}

// HandlerFunc is the signature for configurable endpoint handlers.
type HandlerFunc func(w http.ResponseWriter, r *http.Request)

// recordRequest reads and records a request, then returns it for further use.
func recordRequest(r *http.Request, mu *sync.Mutex, requests *[]RecordedRequest) {
	body, _ := io.ReadAll(r.Body)
	_ = r.Body.Close()
	rec := RecordedRequest{
		Method:  r.Method,
		Path:    r.URL.Path,
		Query:   r.URL.Query(),
		Headers: r.Header.Clone(),
		Body:    body,
	}
	mu.Lock()
	*requests = append(*requests, rec)
	mu.Unlock()
}

// RespondJSON returns a handler that responds with the given status and JSON body.
func RespondJSON(status int, body any) HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		data, _ := json.Marshal(body)
		_, _ = w.Write(data)
	}
}

// RespondStatus returns a handler that responds with just a status code.
func RespondStatus(status int) HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
	}
}

// RespondBytes returns a handler that responds with raw bytes.
func RespondBytes(status int, data []byte) HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write(data)
	}
}

// RespondSequence returns a handler that uses a different handler for each
// successive call. After the last handler is used, it repeats the last one.
func RespondSequence(handlers ...HandlerFunc) HandlerFunc {
	var mu sync.Mutex
	call := 0
	return func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		idx := call
		if idx >= len(handlers) {
			idx = len(handlers) - 1
		}
		call++
		mu.Unlock()
		handlers[idx](w, r)
	}
}
