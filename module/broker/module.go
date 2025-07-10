package broker

import (
	"context"
	"database/sql"

	"github.com/antiloger/Gfy/core"
	"github.com/gin-gonic/gin"
)

type BrokerModule struct {
	gin    *gin.Engine
	db     *sql.DB
	logger core.Logger
}

func NewBrokerModule(gin *gin.Engine, db *sql.DB) *BrokerModule {
	return &BrokerModule{
		gin: gin,
		db:  db,
	}
}

func (b *BrokerModule) Setup(ctx core.AppContext) error {
	b.logger = ctx.Logger()

	ctx.Logger().Info("User module setup completed")
	return nil
}

func (b *BrokerModule) Start(ctx context.Context) error {
	b.gin.GET("/users", b.getUsers)
	b.gin.POST("/push-sse", b.pushSSENotification)
	b.logger.Info("User module started")
	return nil
}

func (b *BrokerModule) Stop(ctx context.Context) error {
	return nil
}

func (b *BrokerModule) Health(ctx context.Context) error {
	return nil
}
