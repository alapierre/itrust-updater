package backend

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestDownloadIdleTimeoutAfterHeaders(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer server.Close()
	b := NewNexusBackend(server.URL, "", "")
	b.TransferIdleTimeout = 100 * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	body, err := b.Get(ctx, "artifact")
	if err != nil {
		t.Fatal(err)
	}
	defer body.Close()
	if _, err := io.ReadAll(body); !errors.Is(err, ErrTransferIdleTimeout) {
		t.Fatalf("expected idle timeout, got %v", err)
	}
	if requests.Load() != 1 {
		t.Fatal("partially read download was retried")
	}
}

func TestDownloadProgressExtendsIdleDeadline(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for i := 0; i < 8; i++ {
			if _, err := w.Write([]byte("x")); err != nil {
				return
			}
			w.(http.Flusher).Flush()
			select {
			case <-time.After(100 * time.Millisecond):
			case <-r.Context().Done():
				return
			}
		}
	}))
	defer server.Close()
	b := NewNexusBackend(server.URL, "", "")
	b.TransferIdleTimeout = 500 * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	body, err := b.Get(ctx, "artifact")
	if err != nil {
		t.Fatal(err)
	}
	defer body.Close()
	data, err := io.ReadAll(body)
	if err != nil || string(data) != strings.Repeat("x", 8) {
		t.Fatalf("active transfer failed: %q, %v", data, err)
	}
}

type transferRoundTripper func(*http.Request) (*http.Response, error)

func (f transferRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestStalledUploadRetriesWithFreshBody(t *testing.T) {
	b := NewNexusBackend("https://example.invalid", "", "")
	b.TransferIdleTimeout = 100 * time.Millisecond
	attempts, opened := 0, 0
	b.Client.Transport = transferRoundTripper(func(r *http.Request) (*http.Response, error) {
		attempts++
		defer r.Body.Close()
		if attempts == 1 {
			// Simulate a transport that stops consuming the upload because its
			// socket write is blocked. It must receive request cancellation.
			var first [1]byte
			if _, err := io.ReadFull(r.Body, first[:]); err != nil {
				return nil, err
			}
			<-r.Context().Done()
			return nil, r.Context().Err()
		}
		data, err := io.ReadAll(r.Body)
		if err != nil || string(data) != "payload" {
			t.Errorf("retry body=%q, err=%v", data, err)
		}
		return &http.Response{StatusCode: http.StatusCreated, Header: make(http.Header), Body: http.NoBody}, nil
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := b.Put(ctx, "artifact", func() (io.ReadCloser, error) {
		opened++
		return io.NopCloser(strings.NewReader("payload")), nil
	}, "application/octet-stream")
	if err != nil || attempts != 2 || opened != 2 {
		t.Fatalf("err=%v, attempts=%d, opened=%d", err, attempts, opened)
	}
}

func TestUploadProgressExtendsIdleDeadline(t *testing.T) {
	b := NewNexusBackend("https://example.invalid", "", "")
	b.TransferIdleTimeout = 500 * time.Millisecond
	b.Client.Transport = transferRoundTripper(func(r *http.Request) (*http.Response, error) {
		defer r.Body.Close()
		var data strings.Builder
		var chunk [1]byte
		for {
			n, err := r.Body.Read(chunk[:])
			data.Write(chunk[:n])
			if err == io.EOF {
				break
			}
			if err != nil {
				return nil, err
			}
			select {
			case <-time.After(100 * time.Millisecond):
			case <-r.Context().Done():
				return nil, r.Context().Err()
			}
		}
		if data.String() != "12345678" {
			t.Errorf("unexpected upload: %q", data.String())
		}
		return &http.Response{StatusCode: http.StatusCreated, Header: make(http.Header), Body: http.NoBody}, nil
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := b.Put(ctx, "artifact", func() (io.ReadCloser, error) {
		return io.NopCloser(strings.NewReader("12345678")), nil
	}, "application/octet-stream")
	if err != nil {
		t.Fatal(err)
	}
}

func TestDownloadParentCancellationAndClose(t *testing.T) {
	for _, closeBody := range []bool{false, true} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.(http.Flusher).Flush()
			<-r.Context().Done()
		}))
		b := NewNexusBackend(server.URL, "", "")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		body, err := b.Get(ctx, "artifact")
		if err != nil {
			cancel()
			server.Close()
			t.Fatal(err)
		}
		if closeBody {
			err = body.Close()
		} else {
			cancel()
			_, err = io.ReadAll(body)
			body.Close()
		}
		cancel()
		server.Close()
		if closeBody && err != nil {
			t.Fatalf("close failed: %v", err)
		}
		if !closeBody && !errors.Is(err, context.Canceled) {
			t.Fatalf("parent cancellation lost: %v", err)
		}
	}
}
