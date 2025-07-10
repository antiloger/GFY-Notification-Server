// core/err.go
package core

import (
	"errors"
	"fmt"
)

// Framework errors - structured and actionable
var (
	ErrComponentNotFound    = errors.New("component not found")
	ErrComponentNotExternal = errors.New("component is not an external")
	ErrComponentNotModule   = errors.New("component is not a module")
	ErrComponentFailed      = errors.New("component operation failed")
	ErrInvalidType          = errors.New("invalid component type")
)

type ComponentError struct {
	Component string
	Operation string
	Err       error
}

func (e ComponentError) Error() string {
	return fmt.Sprintf("component '%s' %s failed: %v", e.Component, e.Operation, e.Err)
}

func (e ComponentError) Unwrap() error {
	return e.Err
}
