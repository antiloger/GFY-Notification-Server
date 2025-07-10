// Package sse provides a module for handling Server-Sent Events (SSE).
package sse

import "github.com/antiloger/Gfy/core"

type SSEModule struct {
	logger core.Logger
}

func NewSSEModule() *SSEModule {
	return &SSEModule{}
}

func (s *SSEModule) Setup(ctx core.AppContext) error {
	return nil
}

func (s *SSEModule) Start(ctx core.AppContext) error {
	return nil
}

func (s *SSEModule) Stop(ctx core.AppContext) error {
	return nil
}

func (s *SSEModule) Health(ctx core.AppContext) error {
	return nil
}
