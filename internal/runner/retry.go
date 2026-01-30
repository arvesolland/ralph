package runner

import (
	"context"
	"errors"
	"math/rand"
	"net"
	"strings"
	"syscall"
	"time"

	"github.com/arvesolland/ralph/internal/log"
)

// RetryConfig holds configuration for retry behavior.
type RetryConfig struct {
	MaxRetries   int           // Maximum number of retry attempts (default 5)
	InitialDelay time.Duration // Initial delay between retries (default 5s)
	MaxDelay     time.Duration // Maximum delay between retries (default 60s)
	JitterFactor float64       // Jitter factor as fraction (default 0.25 for ±25%)
}

// DefaultRetryConfig returns the default retry configuration.
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries:   5,
		InitialDelay: 5 * time.Second,
		MaxDelay:     60 * time.Second,
		JitterFactor: 0.25,
	}
}

// Retrier handles retry logic with exponential backoff.
type Retrier struct {
	config RetryConfig
	clock  Clock // for testing
}

// Clock interface for time operations (allows mocking in tests).
type Clock interface {
	Sleep(d time.Duration)
	Now() time.Time
}

// realClock implements Clock using actual time functions.
type realClock struct{}

func (realClock) Sleep(d time.Duration) { time.Sleep(d) }
func (realClock) Now() time.Time        { return time.Now() }

// NewRetrier creates a new Retrier with the given configuration.
func NewRetrier(config RetryConfig) *Retrier {
	return &Retrier{
		config: config,
		clock:  realClock{},
	}
}

// NewRetrierWithClock creates a new Retrier with a custom clock (for testing).
func NewRetrierWithClock(config RetryConfig, clock Clock) *Retrier {
	return &Retrier{
		config: config,
		clock:  clock,
	}
}

// Do executes the function with retry logic.
// Returns nil if the function succeeds, or the last error if all retries fail.
func (r *Retrier) Do(fn func() error) error {
	return r.DoWithContext(context.Background(), fn)
}

// DoWithContext executes the function with retry logic and context support.
// The context can be used to cancel retries early.
func (r *Retrier) DoWithContext(ctx context.Context, fn func() error) error {
	var lastErr error

	for attempt := 0; attempt <= r.config.MaxRetries; attempt++ {
		// Check context before attempting
		if ctx.Err() != nil {
			if lastErr != nil {
				return lastErr
			}
			return ctx.Err()
		}

		err := fn()
		if err == nil {
			return nil // Success
		}

		lastErr = err

		// Don't retry if error is not retryable
		if !IsRetryable(err) {
			log.Debug("Error is not retryable: %v", err)
			return err
		}

		// Don't retry if we've exhausted attempts
		if attempt >= r.config.MaxRetries {
			log.Debug("Max retries (%d) exhausted", r.config.MaxRetries)
			break
		}

		// Calculate delay with exponential backoff
		delay := r.calculateDelay(attempt)

		log.Info("Retry attempt %d/%d after %v (error: %v)", attempt+1, r.config.MaxRetries, delay, err)

		// Wait with context cancellation support
		select {
		case <-ctx.Done():
			return lastErr
		case <-time.After(delay):
			// Continue to next attempt
		}
	}

	return lastErr
}

// calculateDelay computes the delay for a given attempt using exponential backoff with jitter.
func (r *Retrier) calculateDelay(attempt int) time.Duration {
	// Exponential backoff: initialDelay * 2^attempt
	delay := float64(r.config.InitialDelay) * float64(int(1)<<attempt)

	// Apply jitter (±jitterFactor)
	jitterRange := delay * r.config.JitterFactor
	jitter := (rand.Float64()*2 - 1) * jitterRange // Random value between -jitterRange and +jitterRange
	delay = delay + jitter

	// Ensure delay doesn't go below zero
	if delay < 0 {
		delay = 0
	}

	// Cap at max delay
	if delay > float64(r.config.MaxDelay) {
		delay = float64(r.config.MaxDelay)
	}

	return time.Duration(delay)
}

// Attempts returns the maximum number of attempts (initial + retries).
func (r *Retrier) Attempts() int {
	return r.config.MaxRetries + 1
}

// Common retryable error types
var (
	// ErrRateLimit indicates a rate limit was hit
	ErrRateLimit = errors.New("rate limit exceeded")

	// ErrConnectionFailed indicates a connection failure
	ErrConnectionFailed = errors.New("connection failed")

	// ErrTimeout indicates a timeout
	ErrTimeout = errors.New("operation timed out")
)

// NonRetryableError wraps an error to indicate it should not be retried.
type NonRetryableError struct {
	Err error
}

func (e *NonRetryableError) Error() string {
	return e.Err.Error()
}

func (e *NonRetryableError) Unwrap() error {
	return e.Err
}

// WrapNonRetryable wraps an error to mark it as non-retryable.
func WrapNonRetryable(err error) error {
	if err == nil {
		return nil
	}
	return &NonRetryableError{Err: err}
}

// IsRetryable determines if an error is retryable.
// Returns true for transient errors that may succeed on retry.
func IsRetryable(err error) bool {
	if err == nil {
		return false
	}

	// Check for explicitly non-retryable errors
	var nonRetryable *NonRetryableError
	if errors.As(err, &nonRetryable) {
		return false
	}

	// Context deadline exceeded is retryable (timeout)
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}

	// Context canceled is NOT retryable (user cancellation)
	if errors.Is(err, context.Canceled) {
		return false
	}

	// Custom retryable error types
	if errors.Is(err, ErrRateLimit) || errors.Is(err, ErrConnectionFailed) || errors.Is(err, ErrTimeout) {
		return true
	}

	// Network errors are generally retryable
	var netErr net.Error
	if errors.As(err, &netErr) {
		return netErr.Timeout() || netErr.Temporary()
	}

	// DNS errors are retryable
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return true
	}

	// Connection refused is retryable
	if errors.Is(err, syscall.ECONNREFUSED) || errors.Is(err, syscall.ECONNRESET) {
		return true
	}

	// Check error message for common transient patterns
	errMsg := strings.ToLower(err.Error())

	// Rate limit patterns
	if strings.Contains(errMsg, "rate limit") ||
		strings.Contains(errMsg, "too many requests") ||
		strings.Contains(errMsg, "429") {
		return true
	}

	// Connection/network patterns
	if strings.Contains(errMsg, "connection refused") ||
		strings.Contains(errMsg, "connection reset") ||
		strings.Contains(errMsg, "network unreachable") ||
		strings.Contains(errMsg, "no such host") ||
		strings.Contains(errMsg, "temporary failure") {
		return true
	}

	// Timeout patterns
	if strings.Contains(errMsg, "timeout") ||
		strings.Contains(errMsg, "timed out") ||
		strings.Contains(errMsg, "deadline exceeded") {
		return true
	}

	// Server error patterns (5xx)
	if strings.Contains(errMsg, "500") ||
		strings.Contains(errMsg, "502") ||
		strings.Contains(errMsg, "503") ||
		strings.Contains(errMsg, "504") ||
		strings.Contains(errMsg, "internal server error") ||
		strings.Contains(errMsg, "bad gateway") ||
		strings.Contains(errMsg, "service unavailable") {
		return true
	}

	// Non-retryable patterns (auth, validation, etc.)
	if strings.Contains(errMsg, "invalid") ||
		strings.Contains(errMsg, "unauthorized") ||
		strings.Contains(errMsg, "forbidden") ||
		strings.Contains(errMsg, "not found") ||
		strings.Contains(errMsg, "bad request") ||
		strings.Contains(errMsg, "401") ||
		strings.Contains(errMsg, "403") ||
		strings.Contains(errMsg, "404") ||
		strings.Contains(errMsg, "400") {
		return false
	}

	// Default: don't retry unknown errors
	return false
}
