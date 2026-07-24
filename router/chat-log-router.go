package router

import (
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/gin-gonic/gin"
)

func registerChatLogRoutes(apiRouter *gin.RouterGroup) {
	chatLogRoute := apiRouter.Group("/chat_logs")
	chatLogRoute.Use(middleware.AdminAuth())
	chatLogRoute.GET("/", controller.AdminGetChatLogs)
	chatLogRoute.GET("/:id", controller.AdminGetChatLogDetail)
}
