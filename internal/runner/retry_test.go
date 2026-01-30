package runner

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"
)

// mockClock implements Clock for testing
type mockClock struct {
	mu        sync.Mutex
	sleepTime time.Duration
	sleeps    []time.Duration
}

func (m *mockClock) Sleep(d time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sleepTime += d
	m.sleeps = append(m.sleeps, d)
}

func (m *mockClock) Now() time.Time {
	return time.Now()
}

func (m *mockClock) TotalSleep() time.Duration {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.sleepTime
}

func (m *mockClock) Sleeps() []time.Duration {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]time.Duration{}, m.sleeps...)
}

func TestDefaultRetryConfig(t *testing.T) {
	cfg := DefaultRetryConfig()

	if cfg.MaxRetries != 5 {
		t.Errorf("MaxRetries = %d, want 5", cfg.MaxRetries)
	}
	if cfg.InitialDelay != 5*time.Second {
		t.Errorf("InitialDelay = %v, want 5s", cfg.InitialDelay)
	}
	if cfg.MaxDelay != 60*time.Second {
		t.Errorf("MaxDelay = %v, want 60s", cfg.MaxDelay)
	}
	if cfg.JitterFactor != 0.25 {
		t.Errorf("JitterFactor = %v, want 0.25", cfg.JitterFactor)
	}
}

func TestRetrier_Do_Success(t *testing.T) {
	r := NewRetrier(DefaultRetryConfig())

	called := 0
	err := r.Do(func() error {
		called++
		return nil
	})

	if err != nil {
		t.Errorf("Do() returned error: %v", err)
	}
	if called != 1 {
		t.Errorf("function called %d times, want 1", called)
	}
}

func TestRetrier_Do_SuccessAfterRetries(t *testing.T) {
	cfg := RetryConfig{
		MaxRetries:   5,
		InitialDelay: 1 * time.Millisecond, // Use very short delays for tests
		MaxDelay:     10 * time.Millisecond,
		JitterFactor: 0,
	}
	r := NewRetrier(cfg)

	called := 0
	err := r.Do(func() error {
		called++
		if called < 3 {
			return ErrConnectionFailed
		}
		return nil
	})

	if err != nil {
		t.Errorf("Do() returned error: %v", err)
	}
	if called != 3 {
		t.Errorf("function called %d times, want 3", called)
	}
}

func TestRetrier_Do_MaxRetriesExhausted(t *testing.T) {
	cfg := RetryConfig{
		MaxRetries:   3,
		InitialDelay: 1 * time.Millisecond,
		MaxDelay:     10 * time.Millisecond,
		JitterFactor: 0,
	}
	r := NewRetrier(cfg)

	called := 0
	testErr := ErrConnectionFailed
	err := r.Do(func() error {
		called++
		return testErr
	})

	if err == nil {
		t.Error("Do() should have returned error")
	}
	if !errors.Is(err, testErr) {
		t.Errorf("Do() returned %v, want %v", err, testErr)
	}
	// Initial attempt + 3 retries = 4 total
	if called != 4 {
		t.Errorf("function called %d times, want 4", called)
	}
}

func TestRetrier_Do_NonRetryableError(t *testing.T) {
	cfg := RetryConfig{
		MaxRetries:   5,
		InitialDelay: 1 * time.Millisecond,
		MaxDelay:     10 * time.Millisecond,
		JitterFactor: 0,
	}
	r := NewRetrier(cfg)

	called := 0
	testErr := WrapNonRetryable(errors.New("auth failure"))
	err := r.Do(func() error {
		called++
		return testErr
	})

	if err == nil {
		t.Error("Do() should have returned error")
	}
	// Should not retry non-retryable errors
	if called != 1 {
		t.Errorf("function called %d times, want 1", called)
	}
}

func TestRetrier_DoWithContext_Cancellation(t *testing.T) {
	cfg := RetryConfig{
		MaxRetries:   5,
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     1 * time.Second,
		JitterFactor: 0,
	}
	r := NewRetrier(cfg)

	ctx, cancel := context.WithCancel(context.Background())

	called := 0
	done := make(chan error)

	go func() {
		done <- r.DoWithContext(ctx, func() error {
			called++
			return ErrConnectionFailed
		})
	}()

	// Cancel after a short delay
	time.Sleep(50 * time.Millisecond)
	cancel()

	err := <-done

	// Should have been cancelled early
	if called > 2 {
		t.Errorf("function called %d times, should have been cancelled early", called)
	}
	if err == nil {
		t.Error("DoWithContext() should have returned error after cancellation")
	}
}

func TestRetrier_ExponentialBackoff(t *testing.T) {
	cfg := RetryConfig{
		MaxRetries:   4,
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     1 * time.Second,
		JitterFactor: 0, // No jitter for predictable tests
	}
	r := NewRetrier(cfg)

	// Calculate expected delays
	expected := []time.Duration{
		100 * time.Millisecond, // attempt 0: 100ms * 2^0 = 100ms
		200 * time.Millisecond, // attempt 1: 100ms * 2^1 = 200ms
		400 * time.Millisecond, // attempt 2: 100ms * 2^2 = 400ms
		800 * time.Millisecond, // attempt 3: 100ms * 2^3 = 800ms
	}

	for i, exp := range expected {
		got := r.calculateDelay(i)
		if got != exp {
			t.Errorf("calculateDelay(%d) = %v, want %v", i, got, exp)
		}
	}
}

func TestRetrier_MaxDelayCaped(t *testing.T) {
	cfg := RetryConfig{
		MaxRetries:   10,
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     500 * time.Millisecond,
		JitterFactor: 0,
	}
	r := NewRetrier(cfg)

	// At attempt 10, exponential would be 100ms * 2^10 = 102.4s
	// But should be capped at 500ms
	delay := r.calculateDelay(10)
	if delay > cfg.MaxDelay {
		t.Errorf("calculateDelay(10) = %v, should be capped at %v", delay, cfg.MaxDelay)
	}
}

func TestRetrier_JitterRange(t *testing.T) {
	cfg := RetryConfig{
		MaxRetries:   5,
		InitialDelay: 1 * time.Second,
		MaxDelay:     60 * time.Second,
		JitterFactor: 0.25, // ±25%
	}
	r := NewRetrier(cfg)

	// Run multiple times to test jitter variance
	minDelay := time.Duration(0.75 * float64(time.Second))
	maxDelay := time.Duration(1.25 * float64(time.Second))

	sawMin := false
	sawMax := false
	sawMiddle := false

	for i := 0; i < 100; i++ {
		delay := r.calculateDelay(0)
		if delay < minDelay || delay > maxDelay {
			t.Errorf("calculateDelay(0) = %v, should be in range [%v, %v]", delay, minDelay, maxDelay)
		}

		// Track distribution
		if delay < time.Duration(0.85*float64(time.Second)) {
			sawMin = true
		} else if delay > time.Duration(1.15*float64(time.Second)) {
			sawMax = true
		} else {
			sawMiddle = true
		}
	}

	// Should see some variance (not all the same value)
	if !sawMin || !sawMax || !sawMiddle {
		t.Log("Warning: jitter distribution may be skewed (sawMin:", sawMin, "sawMax:", sawMax, "sawMiddle:", sawMiddle, ")")
	}
}

func TestRetrier_Attempts(t *testing.T) {
	tests := []struct {
		maxRetries int
		want       int
	}{
		{0, 1},
		{1, 2},
		{5, 6},
		{10, 11},
	}

	for _, tt := range tests {
		cfg := RetryConfig{MaxRetries: tt.maxRetries}
		r := NewRetrier(cfg)
		if got := r.Attempts(); got != tt.want {
			t.Errorf("Attempts() with MaxRetries=%d = %d, want %d", tt.maxRetries, got, tt.want)
		}
	}
}

func TestIsRetryable_ContextDeadlineExceeded(t *testing.T) {
	if !IsRetryable(context.DeadlineExceeded) {
		t.Error("context.DeadlineExceeded should be retryable")
	}
}

func TestIsRetryable_ContextCanceled(t *testing.T) {
	if IsRetryable(context.Canceled) {
		t.Error("context.Canceled should NOT be retryable")
	}
}

func TestIsRetryable_CustomErrors(t *testing.T) {
	tests := []struct {
		err       error
		retryable bool
	}{
		{ErrRateLimit, true},
		{ErrConnectionFailed, true},
		{ErrTimeout, true},
		{nil, false},
	}

	for _, tt := range tests {
		if got := IsRetryable(tt.err); got != tt.retryable {
			t.Errorf("IsRetryable(%v) = %v, want %v", tt.err, got, tt.retryable)
		}
	}
}

func TestIsRetryable_NetworkErrors(t *testing.T) {
	// Test via error messages since syscall errors vary by platform
	connRefused := errors.New("connection refused")
	if !IsRetryable(connRefused) {
		t.Error("connection refused error should be retryable")
	}

	connReset := errors.New("connection reset")
	if !IsRetryable(connReset) {
		t.Error("connection reset error should be retryable")
	}
}

func TestIsRetryable_ErrorMessages(t *testing.T) {
	tests := []struct {
		msg       string
		retryable bool
	}{
		// Retryable
		{"rate limit exceeded", true},
		{"too many requests", true},
		{"error 429", true},
		{"connection refused", true},
		{"connection reset by peer", true},
		{"network unreachable", true},
		{"timeout waiting for response", true},
		{"operation timed out", true},
		{"deadline exceeded", true},
		{"500 internal server error", true},
		{"502 bad gateway", true},
		{"503 service unavailable", true},
		{"504 gateway timeout", true},
		{"internal server error", true},
		{"bad gateway", true},
		{"service unavailable", true},

		// Not retryable
		{"invalid argument", false},
		{"unauthorized", false},
		{"forbidden", false},
		{"not found", false},
		{"bad request", false},
		{"error 401", false},
		{"error 403", false},
		{"error 404", false},
		{"error 400", false},
	}

	for _, tt := range tests {
		err := errors.New(tt.msg)
		if got := IsRetryable(err); got != tt.retryable {
			t.Errorf("IsRetryable(%q) = %v, want %v", tt.msg, got, tt.retryable)
		}
	}
}

func TestIsRetryable_NonRetryableWrapper(t *testing.T) {
	baseErr := ErrConnectionFailed // normally retryable
	wrapped := WrapNonRetryable(baseErr)

	if IsRetryable(wrapped) {
		t.Error("NonRetryableError-wrapped error should NOT be retryable")
	}
}

func TestNonRetryableError_Unwrap(t *testing.T) {
	baseErr := errors.New("base error")
	wrapped := WrapNonRetryable(baseErr)

	if !errors.Is(wrapped, baseErr) {
		t.Error("errors.Is should find base error through NonRetryableError")
	}
}

func TestWrapNonRetryable_Nil(t *testing.T) {
	if WrapNonRetryable(nil) != nil {
		t.Error("WrapNonRetryable(nil) should return nil")
	}
}

// mockNetError implements net.Error for testing
type mockNetError struct {
	timeout   bool
	temporary bool
}

func (e *mockNetError) Error() string   { return "mock network error" }
func (e *mockNetError) Timeout() bool   { return e.timeout }
func (e *mockNetError) Temporary() bool { return e.temporary }

func TestIsRetryable_NetError(t *testing.T) {
	tests := []struct {
		name      string
		err       net.Error
		retryable bool
	}{
		{"timeout", &mockNetError{timeout: true}, true},
		{"temporary", &mockNetError{temporary: true}, true},
		{"both", &mockNetError{timeout: true, temporary: true}, true},
		{"neither", &mockNetError{}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsRetryable(tt.err); got != tt.retryable {
				t.Errorf("IsRetryable() = %v, want %v", got, tt.retryable)
			}
		})
	}
}

func TestIsRetryable_DNSError(t *testing.T) {
	// Test via error message since net.DNSError behavior varies by platform
	dnsErr := errors.New("no such host")
	if !IsRetryable(dnsErr) {
		t.Error("DNS lookup errors should be retryable")
	}

	// Also test temporary failure
	tempErr := errors.New("temporary failure in name resolution")
	if !IsRetryable(tempErr) {
		t.Error("temporary DNS failures should be retryable")
	}
}

func TestRetrier_ZeroRetries(t *testing.T) {
	cfg := RetryConfig{
		MaxRetries:   0,
		InitialDelay: 1 * time.Millisecond,
		MaxDelay:     10 * time.Millisecond,
		JitterFactor: 0,
	}
	r := NewRetrier(cfg)

	called := 0
	err := r.Do(func() error {
		called++
		return ErrConnectionFailed
	})

	if err == nil {
		t.Error("Do() should have returned error")
	}
	// With 0 retries, should only be called once
	if called != 1 {
		t.Errorf("function called %d times, want 1", called)
	}
}

func TestRetrier_IntegrationTiming(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping timing test in short mode")
	}

	cfg := RetryConfig{
		MaxRetries:   2,
		InitialDelay: 50 * time.Millisecond,
		MaxDelay:     200 * time.Millisecond,
		JitterFactor: 0,
	}
	r := NewRetrier(cfg)

	start := time.Now()
	called := 0
	r.Do(func() error {
		called++
		return ErrConnectionFailed
	})
	elapsed := time.Since(start)

	// Expected delays: 50ms + 100ms = 150ms (attempt 0: 50ms, attempt 1: 100ms)
	// Allow for some variance
	minExpected := 140 * time.Millisecond
	maxExpected := 200 * time.Millisecond

	if elapsed < minExpected || elapsed > maxExpected {
		t.Errorf("elapsed time %v not in expected range [%v, %v]", elapsed, minExpected, maxExpected)
	}
}

func TestIsRetryable_WrappedErrors(t *testing.T) {
	// Test that wrapped errors are still detected
	wrapped := fmt.Errorf("operation failed: %w", context.DeadlineExceeded)
	if !IsRetryable(wrapped) {
		t.Error("wrapped context.DeadlineExceeded should be retryable")
	}
}
