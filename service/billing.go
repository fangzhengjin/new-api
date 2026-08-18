package service

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
)

const (
	BillingSourceWallet       = "wallet"
	BillingSourceSubscription = "subscription"
)

// PreConsumeBilling 根据用户计费偏好创建 BillingSession 并执行预扣费。
// 会话存储在 relayInfo.Billing 上，供后续 Settle / Refund 使用。
func PreConsumeBilling(c *gin.Context, preConsumedQuota int, relayInfo *relaycommon.RelayInfo) *types.NewAPIError {
	if relayInfo != nil && relayInfo.QuotaClamp != nil {
		return types.NewErrorWithStatusCode(
			relayInfo.QuotaClamp,
			types.ErrorCodeModelPriceError,
			http.StatusBadRequest,
			types.ErrOptionWithSkipRetry(),
		)
	}
	if preConsumedQuota < 0 {
		return types.NewErrorWithStatusCode(
			fmt.Errorf("pre-consume quota cannot be negative: %d", preConsumedQuota),
			types.ErrorCodeModelPriceError,
			http.StatusBadRequest,
			types.ErrOptionWithSkipRetry(),
		)
	}
	if model.CompanyQuotaModeEnabled() {
		if relayInfo == nil || relayInfo.RequestId == "" {
			return types.NewErrorWithStatusCode(errors.New("公司模式计费请求缺少业务键"), types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
		}
		billingAt := relayInfo.StartTime.Unix()
		if err := model.EnsureQuotaSettlementCycle(billingAt); err != nil {
			return types.NewErrorWithStatusCode(err, types.ErrorCodeUpdateDataError, http.StatusServiceUnavailable, types.ErrOptionWithSkipRetry())
		}
		if err := model.RecordQuotaSettlement(relayInfo.RequestId, relayInfo.UserId, 0, billingAt); err != nil {
			return types.NewErrorWithStatusCode(err, types.ErrorCodeUpdateDataError, http.StatusServiceUnavailable, types.ErrOptionWithSkipRetry())
		}
	}
	session, apiErr := NewBillingSession(c, relayInfo, preConsumedQuota)
	if apiErr != nil {
		return apiErr
	}
	relayInfo.Billing = session
	return nil
}

// ---------------------------------------------------------------------------
// SettleBilling — 后结算辅助函数
// ---------------------------------------------------------------------------

// SettleBilling 执行计费结算。如果 RelayInfo 上有 BillingSession 则通过 session 结算，
// 否则回退到旧的 PostConsumeQuota 路径（兼容按次计费等场景）。
func SettleBilling(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, actualQuota int) error {
	if relayInfo == nil || actualQuota < 0 {
		return fmt.Errorf("invalid billing settlement")
	}
	billingAt := relayInfo.StartTime.Unix()
	if err := model.EnsureQuotaSettlementCycle(billingAt); err != nil {
		return err
	}
	if relayInfo.Billing != nil {
		preConsumed := relayInfo.Billing.GetPreConsumedQuota()
		delta := actualQuota - preConsumed

		if delta > 0 {
			logger.LogInfo(ctx, fmt.Sprintf("预扣费后补扣费：%s（实际消耗：%s，预扣费：%s）",
				logger.FormatQuota(delta),
				logger.FormatQuota(actualQuota),
				logger.FormatQuota(preConsumed),
			))
		} else if delta < 0 {
			logger.LogInfo(ctx, fmt.Sprintf("预扣费后返还扣费：%s（实际消耗：%s，预扣费：%s）",
				logger.FormatQuota(-delta),
				logger.FormatQuota(actualQuota),
				logger.FormatQuota(preConsumed),
			))
		} else {
			logger.LogInfo(ctx, fmt.Sprintf("预扣费与实际消耗一致，无需调整：%s（按次计费）",
				logger.FormatQuota(actualQuota),
			))
		}

		if model.CompanyQuotaModeEnabled() {
			if err := model.RecordQuotaSettlement(relayInfo.RequestId, relayInfo.UserId, int64(actualQuota), billingAt); err != nil {
				return err
			}
		}
		if err := relayInfo.Billing.Settle(actualQuota); err != nil {
			session, _ := relayInfo.Billing.(*BillingSession)
			if model.CompanyQuotaModeEnabled() && (session == nil || !session.isFundingSettled()) {
				if rollbackErr := model.RecordQuotaSettlement(relayInfo.RequestId, relayInfo.UserId, int64(preConsumed), billingAt); rollbackErr != nil {
					return errors.Join(err, fmt.Errorf("结算失败且回滚周期账务失败: %w", rollbackErr))
				}
			}
			return err
		}

		// 发送额度通知（订阅计费使用订阅剩余额度）
		if actualQuota != 0 {
			if relayInfo.BillingSource == BillingSourceSubscription {
				checkAndSendSubscriptionQuotaNotify(relayInfo)
			} else {
				checkAndSendQuotaNotify(relayInfo, actualQuota-preConsumed, preConsumed)
			}
		}
		if !model.CompanyQuotaModeEnabled() {
			return model.RecordQuotaSettlement(relayInfo.RequestId, relayInfo.UserId, int64(actualQuota), billingAt)
		}
		return nil
	}

	// 回退：无 BillingSession 时使用旧路径
	quotaDelta := actualQuota - relayInfo.FinalPreConsumedQuota
	if quotaDelta != 0 {
		if err := PostConsumeQuota(relayInfo, quotaDelta, relayInfo.FinalPreConsumedQuota, true); err != nil {
			return err
		}
	}
	return model.RecordQuotaSettlement(relayInfo.RequestId, relayInfo.UserId, int64(actualQuota), billingAt)
}
