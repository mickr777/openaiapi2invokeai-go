package invoke

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestPasswordAuthAddsBearerToken(t *testing.T) {
	var loginCalls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/auth/login":
			loginCalls.Add(1)
			var body struct {
				Email      string `json:"email"`
				Password   string `json:"password"`
				RememberMe bool   `json:"remember_me"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body.Email != "alice@example.test" || body.Password != "secret" || body.RememberMe {
				t.Fatalf("unexpected login payload: %+v", body)
			}
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"token":"jwt-one"}`)
		case "/api/v1/queue/default/i/1":
			if got := r.Header.Get("Authorization"); got != "Bearer jwt-one" {
				t.Fatalf("authorization = %q", got)
			}
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"item_id":1,"status":"completed"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := NewClientWithOptions(srv.URL, time.Second, testLogger(), ClientOptions{
		AuthMode: "password",
		Email: "alice@example.test",
		Password: "secret",
	})
	if _, err := c.GetQueueItemStatus(t.Context(), 1); err != nil {
		t.Fatal(err)
	}
	if loginCalls.Load() != 1 {
		t.Fatalf("login calls = %d", loginCalls.Load())
	}
}

func TestPasswordAuthRetriesOnceAfter401(t *testing.T) {
	var loginCalls atomic.Int32
	var queueCalls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/auth/login":
			n := loginCalls.Add(1)
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"token":"jwt-%d"}`, n)
		case "/api/v1/queue/default/i/1":
			n := queueCalls.Add(1)
			if n == 1 {
				if r.Header.Get("Authorization") != "Bearer jwt-1" {
					t.Fatalf("first token = %q", r.Header.Get("Authorization"))
				}
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			if r.Header.Get("Authorization") != "Bearer jwt-2" {
				t.Fatalf("second token = %q", r.Header.Get("Authorization"))
			}
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"item_id":1,"status":"completed"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := NewClientWithOptions(srv.URL, time.Second, testLogger(), ClientOptions{
		AuthMode: "password",
		Email: "alice@example.test",
		Password: "secret",
	})
	if _, err := c.GetQueueItemStatus(t.Context(), 1); err != nil {
		t.Fatal(err)
	}
	if loginCalls.Load() != 2 || queueCalls.Load() != 2 {
		t.Fatalf("login=%d queue=%d", loginCalls.Load(), queueCalls.Load())
	}
}

func TestLoginFailureDoesNotExposePasswordOrResponseBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "backend-secret-detail", http.StatusUnauthorized)
	}))
	defer srv.Close()

	c := NewClientWithOptions(srv.URL, time.Second, testLogger(), ClientOptions{
		AuthMode: "password",
		Email: "alice@example.test",
		Password: "super-secret-password",
	})
	_, err := c.GetQueueItemStatus(t.Context(), 1)
	if err == nil {
		t.Fatal("expected login error")
	}
	msg := err.Error()
	if strings.Contains(msg, "super-secret-password") || strings.Contains(msg, "backend-secret-detail") {
		t.Fatalf("sensitive login detail leaked: %s", msg)
	}
}

func TestVerifyVersionAutoAndManual(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"version":"v7.0.0rc1"}`)
	}))
	defer srv.Close()

	auto := NewClientWithOptions(srv.URL, time.Second, testLogger(), ClientOptions{Version: "auto"})
	major, err := auto.VerifyVersion(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if major != "7" {
		t.Fatalf("major = %q", major)
	}
	if _, err := auto.VerifyVersion(t.Context()); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 {
		t.Fatalf("auto probe calls = %d", calls.Load())
	}

	manual := NewClientWithOptions("http://127.0.0.1:1", time.Second, testLogger(), ClientOptions{Version: "6"})
	major, err = manual.VerifyVersion(t.Context())
	if err != nil || major != "6" {
		t.Fatalf("manual version = %q, err=%v", major, err)
	}
}
