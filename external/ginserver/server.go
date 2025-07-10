// Package ginserver
package ginserver

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/antiloger/Gfy/config"
	"github.com/antiloger/Gfy/core"
	"github.com/gin-gonic/gin"
)

type GinConfig struct {
	Port         string `env:"GIN_PORT" default:"8080"`
	ReadTimeout  int    `env:"GIN_READTIMEOUT" default:"30"`
	WriteTimeout int    `env:"GIN_WRITETIMEOUT" default:"30"`
	IdealTimeout int    `env:"GIN_IDEALTIMEOUT" default:"120"`
	Environment  string `env:"ENVIRONMENT" default:"debug"`
}

type GinServer struct {
	server *http.Server
	gin    *gin.Engine
	logger core.Logger
	cfg    GinConfig
}

func NewGinServer() *GinServer {
	return &GinServer{
		gin: gin.Default(),
	}
}

func (g *GinServer) Setup(ctx core.AppContext) error {
	g.cfg = config.LoadConfig[GinConfig]()
	g.logger = ctx.Logger()

	if g.cfg.Environment == "production" {
		gin.SetMode(gin.ReleaseMode)
	}
	gin.SetMode(gin.DebugMode)

	g.gin.Use(gin.Logger())
	g.gin.Use(gin.Recovery())
	g.server = &http.Server{
		Addr:         ":" + g.cfg.Port,
		Handler:      g.gin,
		ReadTimeout:  time.Duration(g.cfg.ReadTimeout) * time.Second,
		WriteTimeout: time.Duration(g.cfg.WriteTimeout) * time.Second, // Convert seconds to nanoseconds
	}

	g.logger.Info("Gin server Setup on port: " + g.cfg.Port)

	return nil
}

func (g *GinServer) Start(ctx context.Context) error {
	if g.server == nil {
		return errors.New("server not initialized")
	}

	errCh := make(chan error, 1)
	go func() {
		if err := g.server.ListenAndServe(); err != nil && err == http.ErrServerClosed {
			g.logger.Error("gin server error", core.Field{
				Key:   "err",
				Value: err,
			})
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancle := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancle()
		return g.server.Shutdown(shutdownCtx)
	case err := <-errCh:
		return fmt.Errorf("server failed to start: %w", err)
	}
}

func (g *GinServer) Stop(ctx context.Context) error {
	if g.server == nil {
		return errors.New("server error")
	}

	shutdownCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := g.server.Shutdown(shutdownCtx); err != nil {
		g.logger.Error("gin server shutdown error", core.Field{
			Key:   "err",
			Value: err,
		})
		cancel()
		return err
	}
	g.logger.Info("Gin server stopped gracefully")
	return nil
}

func (g *GinServer) Health(ctx context.Context) error {
	if g.server == nil {
		return errors.New("server not initialized")
	}
	//
	// client := &http.Client{
	// 	Timeout: time.Duration(g.cfg.IdealTimeout) * time.Second,
	// }
	//
	// resp, err := client.Get("http://" + g.server.Addr + "/health")
	// if err != nil {
	// 	g.logger.Error("Health check failed", core.Field{
	// 		Key:   "err",
	// 		Value: err,
	// 	})
	// 	return err
	// }
	// defer resp.Body.Close()
	//
	// if resp.StatusCode != http.StatusOK {
	// 	return errors.New("health check failed with status: " + resp.Status)
	// }

	g.logger.Info("Health check passed")
	return nil
}

func (g *GinServer) GetGin() *gin.Engine {
	return g.gin
}
