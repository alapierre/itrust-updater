package backend

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNexusBackend_PutRetry(t *testing.T) {
	attempts := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		body, _ := io.ReadAll(r.Body)
		if string(body) == "data" {
			w.WriteHeader(http.StatusCreated)
		} else {
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	defer ts.Close()

	ctx := context.Background()
	b := NewNexusBackend(ts.URL, "user", "pass")

	openBody := func() (io.ReadCloser, error) {
		return io.NopCloser(strings.NewReader("data")), nil
	}

	err := b.Put(ctx, "upload.txt", openBody, "text/plain")
	if err != nil {
		t.Fatalf("Put failed after retry: %v", err)
	}

	if attempts != 2 {
		t.Errorf("Expected 2 attempts, got %d", attempts)
	}
}

func TestNexusBackend_GetRetry(t *testing.T) {
	attempts := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("hello"))
	}))
	defer ts.Close()

	ctx := context.Background()
	b := NewNexusBackend(ts.URL, "user", "pass")

	r, err := b.Get(ctx, "test.txt")
	if err != nil {
		t.Fatalf("Get failed after retry: %v", err)
	}
	content, _ := io.ReadAll(r)
	r.Close()
	if string(content) != "hello" {
		t.Errorf("Expected 'hello', got %s", string(content))
	}

	if attempts != 2 {
		t.Errorf("Expected 2 attempts, got %d", attempts)
	}
}

func TestNexusBackend(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" && r.URL.Path == "/test.txt" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("hello"))
			return
		}
		if r.Method == "PUT" && r.URL.Path == "/upload.txt" {
			body, _ := io.ReadAll(r.Body)
			if string(body) == "data" {
				w.WriteHeader(http.StatusCreated)
			} else {
				w.WriteHeader(http.StatusBadRequest)
			}
			return
		}
		if r.Method == "HEAD" && r.URL.Path == "/exists.txt" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	ctx := context.Background()
	b := NewNexusBackend(ts.URL, "user", "pass")

	// Test Get
	r, err := b.Get(ctx, "test.txt")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	content, _ := io.ReadAll(r)
	r.Close()
	if string(content) != "hello" {
		t.Errorf("Expected 'hello', got %s", string(content))
	}

	// Test Put
	openBody := func() (io.ReadCloser, error) {
		return io.NopCloser(strings.NewReader("data")), nil
	}
	err = b.Put(ctx, "upload.txt", openBody, "text/plain")
	if err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	// Test Exists
	exists, err := b.Exists(ctx, "exists.txt")
	if err != nil {
		t.Fatalf("Exists failed: %v", err)
	}
	if !exists {
		t.Error("Expected exists.txt to exist")
	}
}
