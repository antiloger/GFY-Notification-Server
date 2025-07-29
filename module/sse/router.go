package sse

func (s *SSEModule) RegisterRouter() {
	router := s.g.Group("sse")
	router.GET("/notify-connect", s.NotifyConnect)
}
