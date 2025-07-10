package broker

import "github.com/gin-gonic/gin"

func (b *BrokerModule) getUsers(c *gin.Context) {
	c.JSON(200, gin.H{"message": "Get all users"})
}
