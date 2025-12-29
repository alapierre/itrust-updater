package backend

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/alapierre/itrust-updater/pkg/logging"
	"github.com/cenkalti/backoff/v4"
)

var logger = logging.Component("pkg/backend")

type NexusBackend struct {
	BaseURL  string
	Username string
	Password string
	Client   *http.Client
}

func NewNexusBackend(baseURL, username, password string) *NexusBackend {
	return &NexusBackend{
		BaseURL:  strings.TrimSuffix(baseURL, "/"),
		Username: username,
		Password: password,
		Client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (n *NexusBackend) executeWithRetry(ctx context.Context, method, url string, openBody func() (io.ReadCloser, error), contentType string) (*http.Response, error) {
	expBackoff := backoff.NewExponentialBackOff()
	expBackoff.MaxElapsedTime = 30 * time.Second

	var attempt int
	var resp *http.Response

	operation := func() error {
		attempt++
		var body io.ReadCloser
		var err error
		if openBody != nil {
			body, err = openBody()
			if err != nil {
				return backoff.Permanent(err)
			}
			defer body.Close()
		}

		req, err := http.NewRequestWithContext(ctx, method, url, body)
		if err != nil {
			return backoff.Permanent(err)
		}

		if n.Username != "" {
			req.SetBasicAuth(n.Username, n.Password)
		}
		if contentType != "" {
			req.Header.Set("Content-Type", contentType)
		}

		resp, err = n.Client.Do(req)
		if err != nil {
			if isRetryableError(err) {
				logger.Debugf("Retrying %s %s, attempt %d, error: %v", method, url, attempt, err)
				return err
			}
			return backoff.Permanent(err)
		}

		if isRetryableStatus(resp.StatusCode) {
			logger.Debugf("Retrying %s %s, attempt %d, status: %d", method, url, attempt, resp.StatusCode)
			resp.Body.Close()
			return fmt.Errorf("server error: %d", resp.StatusCode)
		}

		return nil
	}

	err := backoff.Retry(operation, backoff.WithContext(expBackoff, ctx))
	if err != nil {
		return nil, err
	}

	return resp, nil
}

func isRetryableError(err error) bool {
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	if strings.Contains(err.Error(), "connection refused") ||
		strings.Contains(err.Error(), "connection reset") ||
		strings.Contains(err.Error(), "EOF") {
		return true
	}
	return false
}

func isRetryableStatus(code int) bool {
	return code >= 500 || code == http.StatusTooManyRequests || code == http.StatusRequestTimeout
}

func (n *NexusBackend) Get(ctx context.Context, path string) (io.ReadCloser, error) {
	url := n.BaseURL + "/" + strings.TrimPrefix(path, "/")
	resp, err := n.executeWithRetry(ctx, "GET", url, nil, "")
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("failed to get %s: %s", url, resp.Status)
	}

	return resp.Body, nil
}

func (n *NexusBackend) Put(ctx context.Context, path string, openBody func() (io.ReadCloser, error), contentType string) error {
	url := n.BaseURL + "/" + strings.TrimPrefix(path, "/")
	resp, err := n.executeWithRetry(ctx, "PUT", url, openBody, contentType)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("failed to put %s: %s", url, resp.Status)
	}

	return nil
}

func (n *NexusBackend) Exists(ctx context.Context, path string) (bool, error) {
	url := n.BaseURL + "/" + strings.TrimPrefix(path, "/")
	resp, err := n.executeWithRetry(ctx, "HEAD", url, nil, "")
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		return true, nil
	}
	if resp.StatusCode == http.StatusNotFound {
		return false, nil
	}
	return false, fmt.Errorf("failed to check existence of %s: %s", url, resp.Status)
}
