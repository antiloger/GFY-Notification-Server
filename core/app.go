// core/app.go - Sync-first with optional Go functions
package core

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Component metadata for tracking
type componentInfo struct {
	name         string
	component    interface{}
	status       ComponentStatus
	isExternal   bool
	lastError    error
	config       ComponentConfig
	retryManager *RetryManager
	mu           sync.RWMutex
}

func (ci *componentInfo) setStatus(status ComponentStatus) {
	ci.mu.Lock()
	defer ci.mu.Unlock()
	ci.status = status
}

func (ci *componentInfo) setError(err error) {
	ci.mu.Lock()
	defer ci.mu.Unlock()
	ci.lastError = err
	if err != nil {
		ci.status = StatusFailed
	}
}

func (ci *componentInfo) getStatus() (ComponentStatus, error) {
	ci.mu.RLock()
	defer ci.mu.RUnlock()
	return ci.status, ci.lastError
}

type Framework struct {
	components    map[string]*componentInfo
	mu            sync.RWMutex
	ctx           context.Context
	cancel        context.CancelFunc
	logger        Logger
	healthChecker *HealthChecker
}

// New creates a new framework instance
func New() *Framework {
	ctx, cancel := context.WithCancel(context.Background())

	fw := &Framework{
		components:    make(map[string]*componentInfo),
		ctx:           ctx,
		cancel:        cancel,
		logger:        newDefaultLogger(),
		healthChecker: newHealthChecker(),
	}

	PrintGrootASCII() // Print ASCII art on startup

	return fw
}

// RegisterExternal registers an external component (infrastructure)
// config is optional - if nil, uses DefaultRetryConfig()
func (f *Framework) RegisterExternal(name string, external External, config ...ComponentConfig) error {
	if external == nil {
		return fmt.Errorf("external component cannot be nil")
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	if _, exists := f.components[name]; exists {
		return fmt.Errorf("component '%s' already registered", name)
	}

	// Use provided config or default
	var componentConfig ComponentConfig
	if len(config) > 0 {
		componentConfig = config[0]
	} else {
		componentConfig = ComponentConfig{
			Retry: DefaultRetryConfig(),
		}
	}

	// Create retry manager for external components
	retryManager := newRetryManager(componentConfig.Retry, f.logger.WithComponent(name))

	info := &componentInfo{
		name:         name,
		component:    external,
		status:       StatusRegistered,
		isExternal:   true,
		config:       componentConfig,
		retryManager: retryManager,
	}

	f.components[name] = info
	f.healthChecker.components[name] = info

	f.logger.Info("external component registered",
		Field{"component", name},
		Field{"type", "external"},
		Field{"max_attempts", componentConfig.Retry.MaxAttempts})

	return nil
}

// RegisterModule registers a module component (business logic)
func (f *Framework) RegisterModule(name string, module Module) error {
	if module == nil {
		return fmt.Errorf("module component cannot be nil")
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	if _, exists := f.components[name]; exists {
		return fmt.Errorf("component '%s' already registered", name)
	}

	info := &componentInfo{
		name:       name,
		component:  module,
		status:     StatusRegistered,
		isExternal: false,
		config:     ComponentConfig{}, // Modules don't get retry config
	}

	f.components[name] = info
	f.healthChecker.components[name] = info

	f.logger.Info("module component registered",
		Field{"component", name},
		Field{"type", "module"})

	return nil
}

// StartExternal starts an external component with setup
func (f *Framework) StartExternal(name string) error {
	return f.startComponent(name, true)
}

// StartModule starts a module component with setup
func (f *Framework) StartModule(name string) error {
	return f.startComponent(name, false)
}

// startComponent handles the unified start logic with panic recovery
func (f *Framework) startComponent(name string, mustBeExternal bool) error {
	f.mu.RLock()
	info, exists := f.components[name]
	f.mu.RUnlock()

	if !exists {
		return ComponentError{
			Component: name,
			Operation: "start",
			Err:       ErrComponentNotFound,
		}
	}

	// Type validation
	if mustBeExternal && !info.isExternal {
		return ComponentError{
			Component: name,
			Operation: "start",
			Err:       fmt.Errorf("%w: use StartModule() instead", ErrComponentNotExternal),
		}
	}
	if !mustBeExternal && info.isExternal {
		return ComponentError{
			Component: name,
			Operation: "start",
			Err:       fmt.Errorf("%w: use StartExternal() instead", ErrComponentNotModule),
		}
	}

	logger := f.logger.WithComponent(name)

	// Setup phase with panic recovery
	if err := f.safeSetup(name, info, logger); err != nil {
		return err
	}

	// Start phase with panic recovery
	if err := f.safeStart(name, info, logger); err != nil {
		return err
	}

	logger.Info("component started successfully")
	return nil
}

// safeSetup runs Setup() with panic recovery
func (f *Framework) safeSetup(name string, info *componentInfo, logger Logger) (retErr error) {
	defer func() {
		if r := recover(); r != nil {
			err := fmt.Errorf("panic during setup: %v", r)
			info.setError(err)
			retErr = ComponentError{
				Component: name,
				Operation: "setup",
				Err:       err,
			}
			logger.Error("component setup panicked",
				Field{"error", err},
				Field{"panic", r})
		}
	}()

	logger.Info("setting up component")

	appCtx := &appContext{
		ctx:    f.ctx,
		logger: logger,
	}

	var err error
	if info.isExternal {
		err = info.component.(External).Setup(appCtx)
	} else {
		err = info.component.(Module).Setup(appCtx)
	}

	if err != nil {
		wrappedErr := ComponentError{
			Component: name,
			Operation: "setup",
			Err:       err,
		}
		info.setError(wrappedErr)
		logger.Error("component setup failed", Field{"error", err})
		return wrappedErr
	}

	info.setStatus(StatusSetup)
	logger.Debug("component setup completed")
	return nil
}

// safeStart runs Start() with panic recovery and retry for externals
func (f *Framework) safeStart(name string, info *componentInfo, logger Logger) (retErr error) {
	defer func() {
		if r := recover(); r != nil {
			err := fmt.Errorf("panic during start: %v", r)
			info.setError(err)
			retErr = ComponentError{
				Component: name,
				Operation: "start",
				Err:       err,
			}
			logger.Error("component start panicked",
				Field{"error", err},
				Field{"panic", r})
			// Don't log success message after panic
		}
	}()

	logger.Info("starting component")

	var err error

	startFunc := func() error {
		if info.isExternal {
			return info.component.(External).Start(f.ctx)
		} else {
			return info.component.(Module).Start(f.ctx)
		}
	}

	// External components get retry logic
	if info.isExternal {
		err = info.retryManager.Retry(f.ctx, "start", startFunc)
	} else {
		// Modules fail fast - no retries
		err = startFunc()
	}

	if err != nil {
		wrappedErr := ComponentError{
			Component: name,
			Operation: "start",
			Err:       err,
		}
		info.setError(wrappedErr)
		logger.Error("component start failed", Field{"error", err})
		return wrappedErr
	}

	info.setStatus(StatusStarted)
	logger.Debug("component start completed")
	return nil // Success case
}

// Stop gracefully shuts down all components (modules first, then externals)
func (f *Framework) Stop() error {
	f.logger.Info("initiating framework shutdown")

	// Cancel context to signal shutdown
	f.cancel()

	var errors []error

	// Stop modules first (business logic should shut down before infrastructure)
	f.mu.RLock()
	var modules, externals []*componentInfo
	for _, info := range f.components {
		if info.status == StatusStarted {
			if info.isExternal {
				externals = append(externals, info)
			} else {
				modules = append(modules, info)
			}
		}
	}
	f.mu.RUnlock()

	// Stop modules first
	for _, info := range modules {
		if err := f.stopComponent(info); err != nil {
			errors = append(errors, err)
		}
	}

	// Then stop externals
	for _, info := range externals {
		if err := f.stopComponent(info); err != nil {
			errors = append(errors, err)
		}
	}

	if len(errors) > 0 {
		f.logger.Error("framework shutdown completed with errors",
			Field{"error_count", len(errors)})
		return fmt.Errorf("shutdown errors: %v", errors)
	}

	f.logger.Info("framework shutdown completed successfully")
	return nil
}

// stopComponent safely stops a single component
func (f *Framework) stopComponent(info *componentInfo) error {
	logger := f.logger.WithComponent(info.name)

	defer func() {
		if r := recover(); r != nil {
			err := fmt.Errorf("panic during stop: %v", r)
			info.setError(err)
			logger.Error("component stop panicked",
				Field{"error", err},
				Field{"panic", r})
		}
	}()

	logger.Info("stopping component")

	// Create timeout context for stop operation
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var err error
	if info.isExternal {
		err = info.component.(External).Stop(ctx)
	} else {
		err = info.component.(Module).Stop(ctx)
	}

	if err != nil {
		wrappedErr := ComponentError{
			Component: info.name,
			Operation: "stop",
			Err:       err,
		}
		info.setError(wrappedErr)
		logger.Error("component stop failed", Field{"error", err})
		return wrappedErr
	}

	info.setStatus(StatusStopped)
	logger.Info("component stopped successfully")
	return nil
}

// GetStatus returns the current status of a component
func (f *Framework) GetStatus(name string) (ComponentStatus, error) {
	f.mu.RLock()
	info, exists := f.components[name]
	f.mu.RUnlock()

	if !exists {
		return StatusRegistered, ErrComponentNotFound
	}

	status, err := info.getStatus()
	return status, err
}

// ListComponents returns all registered components with their status
func (f *Framework) ListComponents() map[string]ComponentStatus {
	f.mu.RLock()
	defer f.mu.RUnlock()

	result := make(map[string]ComponentStatus)
	for name, info := range f.components {
		status, _ := info.getStatus()
		result[name] = status
	}
	return result
}

// Health returns the health status of a specific component
func (f *Framework) Health(ctx context.Context, name string) error {
	return f.healthChecker.CheckHealth(ctx, name)
}

// HealthAll returns health status of all components
func (f *Framework) HealthAll(ctx context.Context) map[string]error {
	return f.healthChecker.CheckAllHealth(ctx)
}

// IsHealthy returns true if all components are healthy
func (f *Framework) IsHealthy(ctx context.Context) bool {
	results := f.HealthAll(ctx)
	for _, err := range results {
		if err != nil {
			return false
		}
	}
	return true
}

// appContext implementation
type appContext struct {
	ctx    context.Context
	logger Logger
}

func (a *appContext) Logger() Logger           { return a.logger }
func (a *appContext) Context() context.Context { return a.ctx }
