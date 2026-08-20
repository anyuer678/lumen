package observability

import (
	"context"
	"fmt"
	"net/http"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
)

var heartbeatOK int32 = 1

func SetHeartbeatOK(ok bool) {
	if ok {
		atomic.StoreInt32(&heartbeatOK, 1)
	} else {
		atomic.StoreInt32(&heartbeatOK, 0)
	}
}

func IsHeartbeatOK() bool {
	return atomic.LoadInt32(&heartbeatOK) == 1
}

type Heartbeat struct {
	interval  time.Duration
	port      int
	failCount int
	maxFails  int
	logger    *zap.SugaredLogger
}

func NewHeartbeat(interval time.Duration, port int, maxFails int, logger *zap.Logger) *Heartbeat {
	h := &Heartbeat{
		interval: interval,
		port:     port,
		maxFails: maxFails,
	}
	if logger != nil {
		h.logger = logger.Sugar()
	}
	return h
}

func (h *Heartbeat) Run(ctx context.Context) {
	ticker := time.NewTicker(h.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			h.check()
		}
	}
}

func (h *Heartbeat) check() {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://localhost:%d/v1/health", h.port))
	if err != nil {
		h.failCount++
		h.logf("heartbeat check failed (%d/%d): %v", h.failCount, h.maxFails, err)
		if h.failCount >= h.maxFails {
			SetHeartbeatOK(false)
			h.logf("heartbeat: core loop appears dead, marking unhealthy")
		}
		return
	}
	resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		h.failCount = 0
		SetHeartbeatOK(true)
	} else {
		h.failCount++
		h.logf("heartbeat check returned status %d", resp.StatusCode)
	}
}

func (h *Heartbeat) logf(format string, args ...interface{}) {
	if h.logger != nil {
		h.logger.Infof(format, args...)
	}
}
