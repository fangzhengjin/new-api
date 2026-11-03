package quota

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQuotaEmailContentSupportsTemporaryQuota(t *testing.T) {
	item := model.QuotaItem{
		Action: model.QuotaAdjustmentActionTemporaryGrant, Username: "quota-user",
		LogContent: "临时额度已发放",
	}

	subject, htmlContent, err := quotaEmailContent(item)

	require.NoError(t, err)
	assert.Contains(t, subject, "AI临时额度发放通知")
	assert.Contains(t, htmlContent, "临时额度已发放")
	assert.Contains(t, htmlContent, "border-left:4px solid #7c3aed")
}

func TestRetryNotificationsRejectsMissingPlan(t *testing.T) {
	setupQuotaTestDB(t)

	summary := RetryNotifications(999999)

	require.Error(t, summary.Err)
	assert.Contains(t, summary.Error, "record not found")
}
