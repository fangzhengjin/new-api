package quota

import (
	"errors"
	"fmt"
	"html"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"gorm.io/gorm"
)

// NotificationSummary reports only deliveries attempted by the current request.
type NotificationSummary struct {
	LogsSent     int    `json:"logs_sent"`
	LogsFailed   int    `json:"logs_failed"`
	EmailsSent   int    `json:"emails_sent"`
	EmailsFailed int    `json:"emails_failed"`
	Skipped      int    `json:"skipped"`
	Error        string `json:"error,omitempty"`
}

// RetryNotifications converges durable log outbox rows before retrying incomplete email delivery.
func RetryNotifications(planID int) NotificationSummary {
	summary := NotificationSummary{}
	var plan model.QuotaPlan
	if err := model.DB.First(&plan, planID).Error; err != nil {
		summary.Error = err.Error()
		return summary
	}
	if plan.Status != model.QuotaPlanStatusExecuted {
		summary.Error = "只有已执行方案可以重试通知"
		return summary
	}
	var items []model.QuotaItem
	if err := model.DB.Where("plan_id = ?", planID).Order("id").Find(&items).Error; err != nil {
		summary.Error = err.Error()
		return summary
	}
	for _, item := range items {
		if item.LogStatus == model.QuotaNotificationStatusPending || item.LogStatus == model.QuotaNotificationStatusFailed {
			if deliverQuotaLog(planID, item.Id) {
				summary.LogsSent++
			} else {
				summary.LogsFailed++
			}
		}
	}
	for _, item := range items {
		if item.Action == model.QuotaAdjustmentActionReclaim {
			if item.EmailStatus != model.QuotaNotificationStatusSkipped {
				model.DB.Model(&model.QuotaItem{}).Where("id = ?", item.Id).
					Updates(map[string]interface{}{"email_status": model.QuotaNotificationStatusSkipped, "email_error": ""})
			}
			summary.Skipped++
			continue
		}
		if item.EmailStatus == model.QuotaNotificationStatusPending || item.EmailStatus == model.QuotaNotificationStatusFailed {
			if deliverQuotaEmail(planID, item.Id) {
				summary.EmailsSent++
			} else {
				summary.EmailsFailed++
			}
		}
	}
	return summary
}

func deliverQuotaLog(planID int, itemID int) bool {
	delivered := false
	transactionErr := model.DB.Transaction(func(tx *gorm.DB) error {
		var item model.QuotaItem
		if err := model.LockForUpdate(tx).
			Where("id = ? AND plan_id = ? AND log_status IN ?", itemID, planID, []model.QuotaNotificationStatus{
				model.QuotaNotificationStatusPending, model.QuotaNotificationStatusFailed,
			}).First(&item).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}
		logDB := model.LOG_DB
		if logDB == nil {
			return markLogFailure(tx, item.Id, errors.New("日志数据库未初始化"))
		}
		if model.LOG_DB == model.DB {
			logDB = tx
		}
		requestID := quotaLogRequestID(planID, item.Id)
		var existing []model.Log
		if err := logDB.Where("request_id = ?", requestID).Limit(2).Find(&existing).Error; err != nil {
			return markLogFailure(tx, item.Id, err)
		}
		if len(existing) > 0 {
			if len(existing) > 1 {
				return markLogFailure(tx, item.Id, errors.New("日志 request_id 对应多条记录"))
			}
			log := existing[0]
			if log.Type != model.LogTypeManage || log.UserId != item.UserId ||
				log.Username != item.Username || log.Content != item.LogContent {
				return markLogFailure(tx, item.Id, errors.New("日志 request_id 已被其他记录占用"))
			}
		} else {
			createdAt := time.Now().Unix()
			if item.ActualAfterQuota == nil {
				return markLogFailure(tx, item.Id, errors.New("明细尚未记录实际执行结果"))
			}
			if err := logDB.Create(&model.Log{
				UserId: item.UserId, Username: item.Username, CreatedAt: createdAt,
				Type: model.LogTypeManage, Content: item.LogContent, RequestId: requestID,
			}).Error; err != nil {
				return markLogFailure(tx, item.Id, err)
			}
		}
		now := time.Now().Unix()
		if err := tx.Model(&model.QuotaItem{}).Where("id = ?", item.Id).Updates(map[string]interface{}{
			"log_status": model.QuotaNotificationStatusSent, "log_sent_at": now, "log_error": "",
		}).Error; err != nil {
			return err
		}
		delivered = true
		return nil
	})
	if transactionErr != nil {
		common.SysError(fmt.Sprintf("failed to deliver quota log for item %d: %v", itemID, transactionErr))
		_ = model.DB.Model(&model.QuotaItem{}).Where("id = ?", itemID).Updates(map[string]interface{}{
			"log_status": model.QuotaNotificationStatusFailed, "log_error": transactionErr.Error(),
		}).Error
	}
	return delivered
}

func markLogFailure(tx *gorm.DB, itemID int, deliveryErr error) error {
	if err := tx.Model(&model.QuotaItem{}).Where("id = ?", itemID).Updates(map[string]interface{}{
		"log_status": model.QuotaNotificationStatusFailed, "log_error": deliveryErr.Error(),
	}).Error; err != nil {
		return err
	}
	return nil
}

func deliverQuotaEmail(planID int, itemID int) bool {
	delivered := false
	transactionErr := model.DB.Transaction(func(tx *gorm.DB) error {
		var item model.QuotaItem
		if err := model.LockForUpdate(tx).
			Where("id = ? AND plan_id = ? AND email_status IN ?", itemID, planID, []model.QuotaNotificationStatus{
				model.QuotaNotificationStatusPending, model.QuotaNotificationStatusFailed,
			}).First(&item).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}
		if item.Email == "" {
			return markEmailFailure(tx, item.Id, errors.New("用户邮箱地址为空"))
		}
		subject, textContent, htmlContent := quotaEmailContent(item)
		if err := common.SendEmailMultipart(subject, item.Email, textContent, htmlContent); err != nil {
			return markEmailFailure(tx, item.Id, err)
		}
		now := time.Now().Unix()
		if err := tx.Model(&model.QuotaItem{}).Where("id = ?", item.Id).Updates(map[string]interface{}{
			"email_status": model.QuotaNotificationStatusSent, "email_sent_at": now, "email_error": "",
		}).Error; err != nil {
			return err
		}
		delivered = true
		return nil
	})
	if transactionErr != nil {
		common.SysError(fmt.Sprintf("failed to deliver quota email for item %d: %v", itemID, transactionErr))
		_ = model.DB.Model(&model.QuotaItem{}).Where("id = ?", itemID).Updates(map[string]interface{}{
			"email_status": model.QuotaNotificationStatusFailed, "email_error": transactionErr.Error(),
		}).Error
	}
	return delivered
}

func markEmailFailure(tx *gorm.DB, itemID int, deliveryErr error) error {
	if err := tx.Model(&model.QuotaItem{}).Where("id = ?", itemID).Updates(map[string]interface{}{
		"email_status": model.QuotaNotificationStatusFailed, "email_error": deliveryErr.Error(),
	}).Error; err != nil {
		return err
	}
	return nil
}

func quotaEmailContent(item model.QuotaItem) (string, string, string) {
	subjects := map[model.QuotaAdjustmentAction]string{
		model.QuotaAdjustmentActionInitialize: "AI额度发放通知",
		model.QuotaAdjustmentActionIncrease:   "AI额度调增通知",
		model.QuotaAdjustmentActionDecrease:   "AI额度调减通知",
		model.QuotaAdjustmentActionGrant:      "AI额度补发通知",
	}
	accents := map[model.QuotaAdjustmentAction]string{
		model.QuotaAdjustmentActionInitialize: "#2563eb",
		model.QuotaAdjustmentActionIncrease:   "#15803d",
		model.QuotaAdjustmentActionDecrease:   "#b91c1c",
		model.QuotaAdjustmentActionGrant:      "#0f766e",
	}
	name := item.DisplayName
	if name == "" {
		name = item.Username
	}
	greeting := name + "，你好："
	intro := "本次额度调整已完成，下面是本次调整信息："
	textContent := fmt.Sprintf("%s\n\n%s\n\n%s", greeting, intro, item.LogContent)
	escapedLines := make([]string, 0)
	for _, line := range strings.Split(item.LogContent, "\n") {
		escapedLines = append(escapedLines, html.EscapeString(line))
	}
	htmlContent := fmt.Sprintf(`<!doctype html>
<html><head><meta charset="UTF-8"></head>
<body style="margin:0;padding:24px;background:#f5f5f5;color:#222;font-family:Arial,'Microsoft YaHei',sans-serif">
<div style="max-width:680px;margin:0 auto;padding:24px;background:#fff;border:1px solid #ddd;border-radius:8px">
<p>%s</p><p>%s</p>
<div style="padding:16px;background:#fafafa;border-left:4px solid %s;line-height:1.6;white-space:pre-wrap;overflow-wrap:anywhere">%s</div>
</div></body></html>`,
		html.EscapeString(greeting), html.EscapeString(intro), accents[item.Action], strings.Join(escapedLines, "\n"))
	return subjects[item.Action], textContent, htmlContent
}
