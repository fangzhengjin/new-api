package router

import (
	"github.com/gin-gonic/gin"

	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"
)

// registerLocalRoutes mounts the routes of the features maintained only in
// this fork. Keeping them here leaves api-router.go identical to upstream
// except for the single call to this function.
func registerLocalRoutes(apiRouter, selfRoute, adminRoute *gin.RouterGroup) {
	apiRouter.POST("/integrations/launch", middleware.UserAuth(), middleware.CriticalRateLimit(), middleware.DisableCache(), controller.LaunchIntegration)
	apiRouter.OPTIONS("/integrations/exchange", middleware.CORS())
	apiRouter.POST("/integrations/exchange", middleware.CORS(), middleware.CriticalRateLimit(), middleware.DisableCache(), middleware.AnonymousRequestBodyLimit(), controller.ExchangeIntegrationCode)

	selfRoute.GET("/temporary-quota", controller.GetSelfTemporaryQuota)
	selfRoute.GET("/temporary-quota/requests", controller.GetSelfTemporaryQuotaRequests)
	selfRoute.POST("/temporary-quota", middleware.CriticalRateLimit(), controller.SubmitSelfTemporaryQuota)

	adminRoute.GET("/:id/limits", controller.GetUserLimits)
	adminRoute.PUT("/:id/limits", controller.UpdateUserLimits)
	adminRoute.DELETE("/:id/access-sources", controller.RemoveUserAccessSource)
	adminRoute.POST("/:id/access-sources/allow", controller.AllowLatestUserAccessSource)

	quotaManagementRoute := apiRouter.Group("/quota-management")
	quotaManagementRoute.Use(middleware.AdminAuth())
	{
		quotaManagementRoute.GET("/overview", controller.GetQuotaOverview)
		quotaManagementRoute.GET("/cycles", controller.GetQuotaCycles)
		quotaManagementRoute.POST("/cycles", controller.CreateQuotaCycle)
		quotaManagementRoute.GET("/cycles/:id", controller.GetQuotaCycle)
		quotaManagementRoute.PATCH("/cycles/:id", controller.UpdateQuotaCycle)
		quotaManagementRoute.POST("/cycles/:id/close", controller.CloseQuotaCycle)
		quotaManagementRoute.GET("/plans/options", controller.GetQuotaPlanOptions)
		quotaManagementRoute.POST("/plans", controller.GenerateQuotaPlan)
		quotaManagementRoute.GET("/plans/:id", controller.GetQuotaPlan)
		quotaManagementRoute.POST("/plans/:id/execute", controller.ExecuteQuotaPlan)
		quotaManagementRoute.POST("/plans/:id/cancel", controller.CancelQuotaPlan)
		quotaManagementRoute.POST("/plans/:id/regenerate", controller.RegenerateQuotaPlan)
		quotaManagementRoute.POST("/plans/:id/notifications/retry", controller.RetryQuotaPlanNotifications)
		quotaManagementRoute.GET("/temporary-quota-requests", controller.GetTemporaryQuotaRequests)
		quotaManagementRoute.POST("/temporary-quota-requests/:id/approve", controller.ApproveTemporaryQuotaRequest)
		quotaManagementRoute.POST("/temporary-quota-requests/:id/reject", controller.RejectTemporaryQuotaRequest)
	}
}
