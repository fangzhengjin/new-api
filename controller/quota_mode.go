package controller

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

func rejectCompanyQuotaMode(c *gin.Context) bool {
	if !model.CompanyQuotaModeEnabled() {
		return false
	}
	common.ApiError(c, model.ErrCompanyQuotaMode)
	return true
}
