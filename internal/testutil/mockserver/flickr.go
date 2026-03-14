package mockserver

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// FlickrMock is a configurable mock Flickr HTTP server.
type FlickrMock struct {
	Server    *httptest.Server
	APIURL    string
	UploadURL string

	onGetPhotos HandlerFunc
	onGetSizes  HandlerFunc
	onUpload    HandlerFunc
	onDownload  HandlerFunc

	mu       sync.Mutex
	requests []RecordedRequest
}

// NewFlickr creates a new unconfigured Flickr mock. Call builder methods then Start().
func NewFlickr(t *testing.T) *FlickrMock {
	m := &FlickrMock{
		onGetPhotos: defaultHandler("flickr.people.getPhotos"),
		onGetSizes:  defaultHandler("flickr.photos.getSizes"),
		onUpload:    defaultHandler("upload"),
		onDownload:  defaultHandler("download"),
	}
	t.Cleanup(func() {
		if m.Server != nil {
			m.Server.Close()
		}
	})
	return m
}

func defaultHandler(name string) HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "mock endpoint not configured: "+name, http.StatusNotImplemented)
	}
}

// OnGetPhotos sets the handler for flickr.people.getPhotos API calls.
func (m *FlickrMock) OnGetPhotos(h HandlerFunc) *FlickrMock {
	m.onGetPhotos = h
	return m
}

// OnGetSizes sets the handler for flickr.photos.getSizes API calls.
func (m *FlickrMock) OnGetSizes(h HandlerFunc) *FlickrMock {
	m.onGetSizes = h
	return m
}

// OnUpload sets the handler for file upload POSTs.
func (m *FlickrMock) OnUpload(h HandlerFunc) *FlickrMock {
	m.onUpload = h
	return m
}

// OnDownload sets the handler for file download GETs at /download/*.
func (m *FlickrMock) OnDownload(h HandlerFunc) *FlickrMock {
	m.onDownload = h
	return m
}

// Start creates and starts the httptest.Server. Returns m for chaining.
func (m *FlickrMock) Start() *FlickrMock {
	m.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recordRequest(r, &m.mu, &m.requests)

		// Route: upload endpoint
		if strings.HasPrefix(r.URL.Path, "/services/upload/") {
			m.onUpload(w, r)
			return
		}

		// Route: download endpoint
		if strings.HasPrefix(r.URL.Path, "/download/") {
			m.onDownload(w, r)
			return
		}

		// Route: API calls (dispatched by "method" query param)
		method := r.URL.Query().Get("method")
		switch method {
		case "flickr.people.getPhotos":
			m.onGetPhotos(w, r)
		case "flickr.photos.getSizes":
			m.onGetSizes(w, r)
		default:
			http.Error(w, "unknown method: "+method, http.StatusNotFound)
		}
	}))
	m.APIURL = m.Server.URL + "/services/rest/"
	m.UploadURL = m.Server.URL + "/services/upload/"
	return m
}

// Requests returns a copy of all recorded requests.
func (m *FlickrMock) Requests() []RecordedRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]RecordedRequest, len(m.requests))
	copy(out, m.requests)
	return out
}
