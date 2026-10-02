package invoke

import (
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestEnqueueAutoRejectsUnsupportedInvokeVersion(t *testing.T) {
	var queueCalls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/app/version":
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"version":"8.0.0"}`)
		case "/api/v1/queue/default/enqueue_batch":
			queueCalls.Add(1)
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := NewClientWithOptions(srv.URL, time.Second, testLogger(), ClientOptions{Version: "auto"})
	if _, err := c.EnqueueBatch(t.Context(), Graph{"nodes": map[string]any{}}); err == nil {
		t.Fatal("expected unsupported-version error")
	}
	if queueCalls.Load() != 0 {
		t.Fatalf("queue request should not be sent for unsupported version; calls=%d", queueCalls.Load())
	}
}

func TestUploadManualVersionSkipsVersionProbe(t *testing.T) {
	var versionCalls atomic.Int32
	var uploadCalls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/app/version":
			versionCalls.Add(1)
			http.Error(w, "should not probe", http.StatusInternalServerError)
		case "/api/v1/images/upload":
			uploadCalls.Add(1)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			io.WriteString(w, `{"image_name":"input.png"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := NewClientWithOptions(srv.URL, time.Second, testLogger(), ClientOptions{Version: "7"})
	if _, err := c.UploadImage(t.Context(), []byte("not-a-real-image"), "input.png", 0, 0); err != nil {
		t.Fatal(err)
	}
	if versionCalls.Load() != 0 || uploadCalls.Load() != 1 {
		t.Fatalf("version=%d upload=%d", versionCalls.Load(), uploadCalls.Load())
	}
}
