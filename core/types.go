// core/types.go
package core

import (
	"context"
)

// Core component interfaces - simple and focused
type External interface {
	Setup(ctx AppContext) error
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	Health(ctx context.Context) error
}

type Module interface {
	Setup(ctx AppContext) error
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	Health(ctx context.Context) error
}

// AppContext provides utilities to components during Setup()
type AppContext interface {
	Logger() Logger
	Context() context.Context
}

// Component status tracking
type ComponentStatus int

const (
	StatusRegistered ComponentStatus = iota
	StatusSetup
	StatusStarted
	StatusStopped
	StatusFailed
)

func (s ComponentStatus) String() string {
	switch s {
	case StatusRegistered:
		return "registered"
	case StatusSetup:
		return "setup"
	case StatusStarted:
		return "started"
	case StatusStopped:
		return "stopped"
	case StatusFailed:
		return "failed"
	default:
		return "unknown"
	}
}
