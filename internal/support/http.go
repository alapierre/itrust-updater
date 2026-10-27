package support

import (
	"fmt"
	"time"

	"github.com/alapierre/itrust-updater/pkg/backend"
	"github.com/alapierre/itrust-updater/pkg/config"
)

func NewNexusBackend(baseURL, username, password string, cfg config.Config) (*backend.NexusBackend, error) {
	timeout := backend.DefaultTransferIdleTimeout
	if raw := cfg["ITRUST_TRANSFER_IDLE_TIMEOUT"]; raw != "" {
		parsed, err := time.ParseDuration(raw)
		if err != nil || parsed <= 0 {
			return nil, fmt.Errorf("ITRUST_TRANSFER_IDLE_TIMEOUT must be a positive duration, e.g. 2m or 30s")
		}
		timeout = parsed
	}
	b := backend.NewNexusBackend(baseURL, username, password)
	b.TransferIdleTimeout = timeout
	return b, nil
}
