package health

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/config"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/errors"
)

// Result is a health check outcome.
type Result struct {
	Success   bool
	Latency   time.Duration
	Message   string
	CheckType string
}

// Checker verifies deployment health before going live.
type Checker struct {
	cfg config.HealthConfig
}

// NewChecker constructs a health checker.
func NewChecker(cfg config.HealthConfig) *Checker {
	return &Checker{cfg: cfg}
}

// WaitUntilHealthy polls until success threshold or failure threshold exceeded.
func (c *Checker) WaitUntilHealthy(ctx context.Context, host string, port int) ([]Result, error) {
	if c.cfg.SuccessThreshold <= 0 {
		c.cfg.SuccessThreshold = 1
	}
	if c.cfg.FailureThreshold <= 0 {
		c.cfg.FailureThreshold = 3
	}
	if c.cfg.MaxRetries <= 0 {
		c.cfg.MaxRetries = 5
	}

	timeout := c.cfg.StartupTimeout
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	deadline := time.Now().Add(timeout)

	var results []Result
	successes := 0
	failures := 0
	backoff := time.Second

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return results, ctx.Err()
		default:
		}

		r := c.checkOnce(ctx, host, port)
		results = append(results, r)

		if r.Success {
			successes++
			failures = 0
			if successes >= c.cfg.SuccessThreshold {
				return results, nil
			}
		} else {
			failures++
			successes = 0
			if failures >= c.cfg.FailureThreshold {
				return results, errors.New(errors.CodeFailedPrecond, "health: failure threshold exceeded")
			}
		}

		time.Sleep(backoff)
		if backoff < 30*time.Second {
			backoff *= 2
		}
	}
	return results, errors.New(errors.CodeTimeout, "health: startup timeout")
}

func (c *Checker) checkOnce(ctx context.Context, host string, port int) Result {
	start := time.Now()

	// TCP check first
	tcpPort := c.cfg.TCPPort
	if tcpPort <= 0 {
		tcpPort = port
	}
	addr := fmt.Sprintf("%s:%d", host, tcpPort)
	conn, err := (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, "tcp", addr)
	if err != nil {
		return Result{Success: false, Latency: time.Since(start), Message: "tcp unreachable", CheckType: "tcp"}
	}
	_ = conn.Close()

	// HTTP readiness check
	path := c.cfg.HTTPPath
	if path == "" {
		path = "/health"
	}
	url := fmt.Sprintf("http://%s:%d%s", host, port, path)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Result{Success: true, Latency: time.Since(start), Message: "tcp ok", CheckType: "tcp"}
	}
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return Result{Success: true, Latency: time.Since(start), Message: "tcp ok, http pending", CheckType: "readiness"}
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 400 {
		return Result{Success: true, Latency: time.Since(start), Message: "healthy", CheckType: "readiness"}
	}
	return Result{Success: false, Latency: time.Since(start), Message: fmt.Sprintf("http %d", resp.StatusCode), CheckType: "readiness"}
}

// Liveness performs a single liveness probe.
func (c *Checker) Liveness(ctx context.Context, host string, port int) Result {
	return c.checkOnce(ctx, host, port)
}
