package core

import (
	"context"
	"fmt"
	"math"
	"math/rand/v2"
	"sync"
	"time"
)

// Retry configuration for external components
type RetryConfig struct {
	MaxAttempts   int
	InitialDelay  time.Duration
	MaxDelay      time.Duration
	Multiplier    float64
	Jitter        float64
	RetryableFunc func(error) bool // Determines if error is retryable
}

// DefaultRetryConfig provides sensible defaults for external components
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxAttempts:  3,
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     5 * time.Second,
		Multiplier:   2.0,
		Jitter:       0.1,
		RetryableFunc: func(err error) bool {
			// Default: retry on temporary/timeout errors
			if err == nil {
				return false
			}
			// Add more sophisticated error classification here
			return true
		},
	}
}

// ComponentConfig holds configuration for a component
type ComponentConfig struct {
	Retry RetryConfig
}

// RetryManager handles retries for external components
type RetryManager struct {
	config RetryConfig
	logger Logger
}

func newRetryManager(config RetryConfig, logger Logger) *RetryManager {
	return &RetryManager{
		config: config,
		logger: logger,
	}
}

// Retry executes the operation with retry logic
func (rm *RetryManager) Retry(ctx context.Context, operation string, fn func() error) error {
	var lastErr error

	for attempt := 1; attempt <= rm.config.MaxAttempts; attempt++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		err := fn()
		if err == nil {
			if attempt > 1 {
				rm.logger.Info("operation succeeded after retry",
					Field{"operation", operation},
					Field{"attempt", attempt})
			}
			return nil
		}

		lastErr = err

		// Check if error is retryable
		if !rm.config.RetryableFunc(err) {
			rm.logger.Debug("error not retryable, failing immediately",
				Field{"operation", operation},
				Field{"error", err})
			return err
		}

		// Don't delay on last attempt
		if attempt >= rm.config.MaxAttempts {
			break
		}

		delay := rm.calculateDelay(attempt)
		rm.logger.Warn("operation failed, retrying",
			Field{"operation", operation},
			Field{"attempt", attempt},
			Field{"max_attempts", rm.config.MaxAttempts},
			Field{"delay", delay},
			Field{"error", err})

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
	}

	return fmt.Errorf("operation failed after %d attempts: %w", rm.config.MaxAttempts, lastErr)
}

func (rm *RetryManager) calculateDelay(attempt int) time.Duration {
	delay := float64(rm.config.InitialDelay) * math.Pow(rm.config.Multiplier, float64(attempt-1))

	// Apply jitter
	if rm.config.Jitter > 0 {
		jitter := delay * rm.config.Jitter * (rand.Float64()*2 - 1) // -jitter to +jitter
		delay += jitter
	}

	// Cap at max delay
	if maxDelay := float64(rm.config.MaxDelay); delay > maxDelay {
		delay = maxDelay
	}

	return time.Duration(delay)
}

// HealthChecker aggregates health status
type HealthChecker struct {
	components map[string]*componentInfo
	mu         sync.RWMutex
}

func newHealthChecker() *HealthChecker {
	return &HealthChecker{
		components: make(map[string]*componentInfo),
	}
}

// CheckHealth performs health check on a specific component
func (hc *HealthChecker) CheckHealth(ctx context.Context, name string) error {
	hc.mu.RLock()
	info, exists := hc.components[name]
	hc.mu.RUnlock()

	if !exists {
		return fmt.Errorf("component '%s' not found", name)
	}

	status, err := info.getStatus()
	if status != StatusStarted {
		return fmt.Errorf("component '%s' not started (status: %s)", name, status)
	}

	if err != nil {
		return fmt.Errorf("component '%s' has error: %w", name, err)
	}

	// Call component's health check
	if info.isExternal {
		return info.component.(External).Health(ctx)
	} else {
		return info.component.(Module).Health(ctx)
	}
}

// CheckAllHealth performs health check on all started components
func (hc *HealthChecker) CheckAllHealth(ctx context.Context) map[string]error {
	hc.mu.RLock()
	components := make([]*componentInfo, 0, len(hc.components))
	for _, info := range hc.components {
		if status, _ := info.getStatus(); status == StatusStarted {
			components = append(components, info)
		}
	}
	hc.mu.RUnlock()

	results := make(map[string]error)
	for _, info := range components {
		results[info.name] = hc.CheckHealth(ctx, info.name)
	}

	return results
}
