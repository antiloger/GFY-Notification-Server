// Package broker
package broker

import (
	"net/http"

	ntypes "github.com/antiloger/Gfy/pkg/Ntypes"
	"github.com/gin-gonic/gin"
)

func (b *BrokerModule) pushSSENotification(c *gin.Context) {
	var Request ntypes.SSENotificationRequest
	if err := c.BindJSON(&Request); err != nil {
		c.JSON(
			http.StatusBadRequest,
			ntypes.NewErrorResponse(ntypes.BindingError, "Error binding request", err.Error()),
		)
		return
	}
}
