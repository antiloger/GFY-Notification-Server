// Package redisserver provides a setup for a Redis server with a custom configuration.
package redisserver

import (
	"context"
	"fmt"
	"time"

	"github.com/antiloger/Gfy/config"
	"github.com/antiloger/Gfy/core"
	"github.com/redis/go-redis/v9"
)

type RedisServerConfig struct {
	Addr         string `env:"REDIS_ADDR" default:"localhost:6379"`
	Password     string `env:"REDIS_PASSWORD" default:""`
	DB           int    `env:"REDIS_DB" default:"0"`
	PoolSize     int    `env:"REDIS_POOL_SIZE" default:"10"`
	MinIdleConns int    `env:"REDIS_MIN_IDLE_CONNS" default:"5"`
}

type RedisServer struct {
	client *redis.Client
	logger core.Logger
	cfg    RedisServerConfig
}

func NewRedisServer() *RedisServer {
	return &RedisServer{}
}

func (r *RedisServer) Setup(ctx core.AppContext) error {
	r.cfg = config.LoadConfig[RedisServerConfig]()
	r.logger = ctx.Logger()
	return nil
}

func (r *RedisServer) Start(ctx context.Context) error {
	r.client = redis.NewClient(&redis.Options{
		Addr:         r.cfg.Addr,
		Password:     r.cfg.Password,
		DB:           r.cfg.DB,
		PoolSize:     r.cfg.PoolSize,
		MinIdleConns: r.cfg.MinIdleConns,
	})

	// Test the connection
	if err := r.client.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("failed to connect to Redis: %w", err)
	}

	r.logger.Info("Redis server started successfully", core.Field{
		Key:   "addr",
		Value: r.cfg.Addr,
	})

	return nil
}

func (r *RedisServer) Stop(ctx context.Context) error {
	if r.client == nil {
		return nil
	}

	// Close the Redis client connection
	if err := r.client.Close(); err != nil {
		r.logger.Error("Failed to close Redis client", core.Field{
			Key:   "err",
			Value: err,
		})
		return fmt.Errorf("failed to close Redis client: %w", err)
	}

	r.logger.Info("Redis server stopped successfully")
	return nil
}

func (r *RedisServer) Health(ctx context.Context) error {
	if r.client == nil {
		return fmt.Errorf("redis client is not initialized")
	}

	// Create a context with timeout for health check
	healthCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// Ping Redis to check health
	if err := r.client.Ping(healthCtx).Err(); err != nil {
		return fmt.Errorf("redis health check failed: %w", err)
	}

	return nil
}

func (r *RedisServer) GetClient() *redis.Client {
	return r.client
}
