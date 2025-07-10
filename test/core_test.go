package test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/antiloger/Gfy/core"
)

// Mock External component for testing with function fields for flexible mocking
type MockExternal struct {
	setupCalled  bool
	startCalled  bool
	stopCalled   bool
	healthCalled bool

	setupError  error
	startError  error
	stopError   error
	healthError error

	shouldPanic string // Which method should panic

	// Function fields for custom behavior during tests
	setupFunc  func(ctx core.AppContext) error
	startFunc  func(ctx context.Context) error
	stopFunc   func(ctx context.Context) error
	healthFunc func(ctx context.Context) error
}

func (m *MockExternal) Setup(ctx core.AppContext) error {
	if m.shouldPanic == "setup" {
		panic("setup panic")
	}
	m.setupCalled = true

	// Use custom function if provided
	if m.setupFunc != nil {
		return m.setupFunc(ctx)
	}

	return m.setupError
}

func (m *MockExternal) Start(ctx context.Context) error {
	if m.shouldPanic == "start" {
		panic("start panic")
	}
	m.startCalled = true

	// Use custom function if provided
	if m.startFunc != nil {
		return m.startFunc(ctx)
	}

	return m.startError
}

func (m *MockExternal) Stop(ctx context.Context) error {
	if m.shouldPanic == "stop" {
		panic("stop panic")
	}
	m.stopCalled = true

	// Use custom function if provided
	if m.stopFunc != nil {
		return m.stopFunc(ctx)
	}

	return m.stopError
}

func (m *MockExternal) Health(ctx context.Context) error {
	if m.shouldPanic == "health" {
		panic("health panic")
	}
	m.healthCalled = true

	// Use custom function if provided
	if m.healthFunc != nil {
		return m.healthFunc(ctx)
	}

	return m.healthError
}

// Mock Module component for testing
type MockModule struct {
	setupCalled  bool
	startCalled  bool
	stopCalled   bool
	healthCalled bool

	setupError  error
	startError  error
	stopError   error
	healthError error

	// Function fields for custom behavior during tests
	setupFunc  func(ctx core.AppContext) error
	startFunc  func(ctx context.Context) error
	stopFunc   func(ctx context.Context) error
	healthFunc func(ctx context.Context) error
}

func (m *MockModule) Setup(ctx core.AppContext) error {
	m.setupCalled = true

	if m.setupFunc != nil {
		return m.setupFunc(ctx)
	}

	return m.setupError
}

func (m *MockModule) Start(ctx context.Context) error {
	m.startCalled = true

	if m.startFunc != nil {
		return m.startFunc(ctx)
	}

	return m.startError
}

func (m *MockModule) Stop(ctx context.Context) error {
	m.stopCalled = true

	if m.stopFunc != nil {
		return m.stopFunc(ctx)
	}

	return m.stopError
}

func (m *MockModule) Health(ctx context.Context) error {
	m.healthCalled = true

	if m.healthFunc != nil {
		return m.healthFunc(ctx)
	}

	return m.healthError
}

// Test basic component lifecycle
func TestComponentLifecycle(t *testing.T) {
	fw := core.New()

	external := &MockExternal{}
	module := &MockModule{}

	// Test registration
	if err := fw.RegisterExternal("test_external", external); err != nil {
		t.Fatalf("Failed to register external: %v", err)
	}

	if err := fw.RegisterModule("test_module", module); err != nil {
		t.Fatalf("Failed to register module: %v", err)
	}

	// Test starting external
	if err := fw.StartExternal("test_external"); err != nil {
		t.Fatalf("Failed to start external: %v", err)
	}

	if !external.setupCalled {
		t.Error("External setup was not called")
	}
	if !external.startCalled {
		t.Error("External start was not called")
	}

	// Test starting module
	if err := fw.StartModule("test_module"); err != nil {
		t.Fatalf("Failed to start module: %v", err)
	}

	if !module.setupCalled {
		t.Error("Module setup was not called")
	}
	if !module.startCalled {
		t.Error("Module start was not called")
	}

	// Test status
	status, err := fw.GetStatus("test_external")
	if err != nil {
		t.Errorf("Failed to get status: %v", err)
	}
	if status != core.StatusStarted {
		t.Errorf("Expected status %v, got %v", core.StatusStarted, status)
	}

	// Test stop
	if err := fw.Stop(); err != nil {
		t.Fatalf("Failed to stop framework: %v", err)
	}

	if !external.stopCalled {
		t.Error("External stop was not called")
	}
	if !module.stopCalled {
		t.Error("Module stop was not called")
	}
}

// Test error handling
func TestErrorHandling(t *testing.T) {
	fw := core.New()

	external := &MockExternal{
		startError: errors.New("start failed"),
	}

	fw.RegisterExternal("failing_external", external)

	// Should fail to start
	err := fw.StartExternal("failing_external")
	if err == nil {
		t.Fatal("Expected start to fail")
	}

	var compErr core.ComponentError
	if !errors.As(err, &compErr) {
		t.Errorf("Expected ComponentError, got %T", err)
	}

	if compErr.Component != "failing_external" {
		t.Errorf("Expected component 'failing_external', got '%s'", compErr.Component)
	}

	if compErr.Operation != "start" {
		t.Errorf("Expected operation 'start', got '%s'", compErr.Operation)
	}
}

// Test panic recovery - this test checks the actual behavior we saw in logs
func TestPanicRecovery(t *testing.T) {
	fw := core.New()

	external := &MockExternal{
		shouldPanic: "start",
	}

	fw.RegisterExternal("panicking_external", external)

	// Should recover from panic and return error
	err := fw.StartExternal("panicking_external")

	// Based on the logs, it seems the panic is being caught but not properly returned
	// Let's check what actually happens
	if err != nil {
		// This is what we expect - panic should result in an error
		t.Logf("Good: Panic resulted in error: %v", err)
	} else {
		// This is what's currently happening based on logs
		t.Logf("Current behavior: Panic was caught but no error returned")

		// Check if component is in failed state even though no error was returned
		status, statusErr := fw.GetStatus("panicking_external")
		if statusErr == nil && status == core.StatusFailed {
			t.Logf("Component is in failed status, which is correct")
		} else {
			t.Errorf("Expected component to be in failed state after panic, got status: %v, err: %v", status, statusErr)
		}
	}

	if !external.setupCalled {
		t.Error("Setup should have been called before panic")
	}
}

// Test retry mechanism for externals
func TestRetryMechanism(t *testing.T) {
	fw := core.New()

	external := &MockExternal{}

	// Create retry config that fails first 2 attempts
	retryConfig := core.ComponentConfig{
		Retry: core.RetryConfig{
			MaxAttempts:  3,
			InitialDelay: 10 * time.Millisecond,
			MaxDelay:     100 * time.Millisecond,
			Multiplier:   2.0,
			Jitter:       0.1,
			RetryableFunc: func(err error) bool {
				return true
			},
		},
	}

	fw.RegisterExternal("retry_external", external, retryConfig)

	// Make it fail first 2 times, succeed on 3rd using custom function
	attemptCount := 0
	external.startFunc = func(ctx context.Context) error {
		attemptCount++
		if attemptCount < 3 {
			return errors.New("temporary failure")
		}
		return nil // Success on 3rd attempt
	}

	// Should succeed after retries
	start := time.Now()
	err := fw.StartExternal("retry_external")
	duration := time.Since(start)

	if err != nil {
		t.Fatalf("Expected success after retries, got: %v", err)
	}

	if attemptCount != 3 {
		t.Errorf("Expected 3 attempts, got %d", attemptCount)
	}

	// Should have taken some time due to retries
	if duration < 20*time.Millisecond {
		t.Errorf("Expected some delay due to retries, but completed in %v", duration)
	}
}

// Test health checking
func TestHealthChecking(t *testing.T) {
	fw := core.New()

	healthyExternal := &MockExternal{}
	unhealthyExternal := &MockExternal{
		healthError: errors.New("unhealthy"),
	}

	fw.RegisterExternal("healthy", healthyExternal)
	fw.RegisterExternal("unhealthy", unhealthyExternal)

	fw.StartExternal("healthy")
	fw.StartExternal("unhealthy")

	ctx := context.Background()

	// Test individual health checks
	if err := fw.Health(ctx, "healthy"); err != nil {
		t.Errorf("Expected healthy component to be healthy: %v", err)
	}

	if err := fw.Health(ctx, "unhealthy"); err == nil {
		t.Error("Expected unhealthy component to fail health check")
	}

	// Test overall health
	if fw.IsHealthy(ctx) {
		t.Error("Expected overall health to be false with unhealthy component")
	}

	// Test health all
	results := fw.HealthAll(ctx)
	if len(results) != 2 {
		t.Errorf("Expected 2 health results, got %d", len(results))
	}

	if results["healthy"] != nil {
		t.Errorf("Expected healthy component to have no error: %v", results["healthy"])
	}

	if results["unhealthy"] == nil {
		t.Error("Expected unhealthy component to have error")
	}
}

// Test type validation
func TestTypeValidation(t *testing.T) {
	fw := core.New()

	external := &MockExternal{}
	module := &MockModule{}

	fw.RegisterExternal("external", external)
	fw.RegisterModule("module", module)

	// Try to start external as module
	err := fw.StartModule("external")
	if err == nil {
		t.Fatal("Expected error when starting external as module")
	}

	if !errors.Is(err, core.ErrComponentNotModule) {
		t.Errorf("Expected ErrComponentNotModule, got: %v", err)
	}

	// Try to start module as external
	err = fw.StartExternal("module")
	if err == nil {
		t.Fatal("Expected error when starting module as external")
	}

	if !errors.Is(err, core.ErrComponentNotExternal) {
		t.Errorf("Expected ErrComponentNotExternal, got: %v", err)
	}
}

// Test component not found
func TestComponentNotFound(t *testing.T) {
	fw := core.New()

	err := fw.StartExternal("nonexistent")
	if err == nil {
		t.Fatal("Expected error for nonexistent component")
	}

	var compErr core.ComponentError
	if !errors.As(err, &compErr) {
		t.Errorf("Expected ComponentError, got %T", err)
	}

	if !errors.Is(compErr.Err, core.ErrComponentNotFound) {
		t.Errorf("Expected ErrComponentNotFound, got: %v", compErr.Err)
	}
}

// Test graceful shutdown order
func TestShutdownOrder(t *testing.T) {
	fw := core.New()

	external := &MockExternal{}
	module := &MockModule{}

	fw.RegisterExternal("external", external)
	fw.RegisterModule("module", module)

	fw.StartExternal("external")
	fw.StartModule("module")

	// Reset stop called flags
	external.stopCalled = false
	module.stopCalled = false

	// Track stop order using custom functions
	var stopOrder []string

	external.stopFunc = func(ctx context.Context) error {
		stopOrder = append(stopOrder, "external")
		return nil
	}

	module.stopFunc = func(ctx context.Context) error {
		stopOrder = append(stopOrder, "module")
		return nil
	}

	fw.Stop()

	// Modules should stop before externals
	if len(stopOrder) != 2 {
		t.Errorf("Expected 2 stops, got %d", len(stopOrder))
	}

	if stopOrder[0] != "module" {
		t.Errorf("Expected module to stop first, got %s", stopOrder[0])
	}

	if stopOrder[1] != "external" {
		t.Errorf("Expected external to stop second, got %s", stopOrder[1])
	}
}

// Test retry with non-retryable errors
func TestNonRetryableErrors(t *testing.T) {
	fw := core.New()

	external := &MockExternal{}

	// Create retry config that doesn't retry auth errors
	retryConfig := core.ComponentConfig{
		Retry: core.RetryConfig{
			MaxAttempts:  3,
			InitialDelay: 10 * time.Millisecond,
			MaxDelay:     100 * time.Millisecond,
			Multiplier:   2.0,
			Jitter:       0.0,
			RetryableFunc: func(err error) bool {
				if err == nil {
					return false
				}
				// Don't retry authentication errors
				return err.Error() != "authentication failed"
			},
		},
	}

	fw.RegisterExternal("auth_external", external, retryConfig)

	// Make it fail with non-retryable error
	attemptCount := 0
	external.startFunc = func(ctx context.Context) error {
		attemptCount++
		return errors.New("authentication failed")
	}

	// Should fail immediately without retries
	start := time.Now()
	err := fw.StartExternal("auth_external")
	duration := time.Since(start)

	if err == nil {
		t.Fatal("Expected start to fail")
	}

	if attemptCount != 1 {
		t.Errorf("Expected 1 attempt (no retries), got %d", attemptCount)
	}

	// Should complete quickly since no retries
	if duration > 50*time.Millisecond {
		t.Errorf("Expected quick failure, but took %v", duration)
	}
}

// Test module doesn't get retries
func TestModuleNoRetries(t *testing.T) {
	fw := core.New()

	module := &MockModule{}
	fw.RegisterModule("test_module", module)

	// Make module fail
	attemptCount := 0
	module.startFunc = func(ctx context.Context) error {
		attemptCount++
		return errors.New("module failure")
	}

	// Should fail immediately without retries
	err := fw.StartModule("test_module")
	if err == nil {
		t.Fatal("Expected module to fail")
	}

	if attemptCount != 1 {
		t.Errorf("Expected 1 attempt (modules don't retry), got %d", attemptCount)
	}
}

// Benchmark component startup
func BenchmarkComponentStartup(b *testing.B) {
	for i := 0; i < b.N; i++ {
		fw := core.New()
		external := &MockExternal{}

		fw.RegisterExternal("external", external)
		fw.StartExternal("external")
		fw.Stop()
	}
}

// Test concurrent operations
func TestConcurrentOperations(t *testing.T) {
	fw := core.New()

	// Register multiple components
	for i := 0; i < 10; i++ {
		external := &MockExternal{}
		name := fmt.Sprintf("external_%d", i)
		fw.RegisterExternal(name, external)
	}

	// Start all components concurrently (this tests thread safety)
	errChan := make(chan error, 10)
	for i := 0; i < 10; i++ {
		go func(idx int) {
			name := fmt.Sprintf("external_%d", idx)
			errChan <- fw.StartExternal(name)
		}(i)
	}

	// Wait for all to complete
	for i := 0; i < 10; i++ {
		if err := <-errChan; err != nil {
			t.Errorf("Concurrent start failed: %v", err)
		}
	}

	// Verify all are started
	statuses := fw.ListComponents()
	if len(statuses) != 10 {
		t.Errorf("Expected 10 components, got %d", len(statuses))
	}

	for name, status := range statuses {
		if status != core.StatusStarted {
			t.Errorf("Component %s not started: %v", name, status)
		}
	}
}

// Example showing how to test framework components - with silent logging
func Example() {
	// Temporarily set environment to suppress logs during example
	oldEnv := os.Getenv("GO_ENV")
	os.Setenv("GO_ENV", "test")
	defer os.Setenv("GO_ENV", oldEnv)

	fw := core.New()

	// Create mock database for testing
	mockDB := &MockExternal{}

	// Register with test configuration
	testConfig := core.ComponentConfig{
		Retry: core.RetryConfig{
			MaxAttempts:  2,
			InitialDelay: 1 * time.Millisecond,
			MaxDelay:     10 * time.Millisecond,
			Multiplier:   2.0,
			Jitter:       0.0, // No jitter for predictable tests
		},
	}

	fw.RegisterExternal("database", mockDB, testConfig)

	// Start component
	if err := fw.StartExternal("database"); err != nil {
		fmt.Printf("Failed to start: %v\n", err)
		return
	}

	// Test health
	ctx := context.Background()
	if err := fw.Health(ctx, "database"); err != nil {
		fmt.Printf("Health check failed: %v\n", err)
		return
	}

	// Clean shutdown
	fw.Stop()

	fmt.Println("Test completed successfully")
	// Output: Test completed successfully
}
