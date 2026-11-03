/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { getCurrencyDisplay, getCurrencyLabel } from '@/lib/currency'
import { formatQuota, parseQuotaFromDollars } from '@/lib/format'
import { handleServerError } from '@/lib/handle-server-error'
import { cn } from '@/lib/utils'
import { useSystemConfigStore } from '@/stores/system-config-store'

import { adjustUserQuota } from '../api'
import type { ManageUserQuotaPayload, QuotaAdjustMode } from '../types'

interface UserQuotaDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  userId: number
  currentQuota: number
  onSuccess: () => void
}

export function UserQuotaDialog(props: UserQuotaDialogProps) {
  const { t } = useTranslation()
  const [mode, setMode] = useState<QuotaAdjustMode>('add')
  const [amount, setAmount] = useState('')
  const [reason, setReason] = useState('')
  const [amountError, setAmountError] = useState<string | null>(null)
  const [reasonError, setReasonError] = useState<string | null>(null)
  const [loading, setLoading] = useState(false)
  const cycleQuotaManagementEnabled = useSystemConfigStore(
    (state) => state.config.cycleQuotaManagementEnabled === true
  )

  const { meta: currencyMeta } = getCurrencyDisplay()
  const currencyLabel = getCurrencyLabel()
  const tokensOnly = currencyMeta.kind === 'tokens'

  const parsedAmount = Number.parseFloat(amount)
  const amountValue = Number.isFinite(parsedAmount) ? parsedAmount : 0
  const quotaValue = parseQuotaFromDollars(Math.abs(amountValue))

  const getPreviewText = () => {
    const current = props.currentQuota
    const val = quotaValue
    if (
      !amount.trim() ||
      !Number.isFinite(parsedAmount) ||
      (mode !== 'override' && (amountValue <= 0 || quotaValue <= 0))
    ) {
      return t('当前余额：{{current}}', {
        current: formatQuota(current),
      })
    }
    switch (mode) {
      case 'add':
        return t('当前余额：{{current}} → 调增后余额：{{after}}（+{{amount}}）',
          {
            current: formatQuota(current),
            after: formatQuota(current + val),
            amount: formatQuota(val),
          }
        )
      case 'subtract':
        return t('当前余额：{{current}} → 调减后余额：{{after}}（-{{amount}}）',
          {
            current: formatQuota(current),
            after: formatQuota(current - val),
            amount: formatQuota(val),
          }
        )
      case 'override': {
        const overrideQuota = parseQuotaFromDollars(amountValue)
        return t('当前余额：{{current}} → 调整后余额：{{after}}',
          {
            current: formatQuota(current),
            after: formatQuota(overrideQuota),
          }
        )
      }
      default:
        return ''
    }
  }

  const handleOpenChange = (open: boolean) => {
    if (!open) {
      setAmount('')
      setMode('add')
      setReason('')
      setAmountError(null)
      setReasonError(null)
    }
    props.onOpenChange(open)
  }

  const submitAdjustment = async () => {
    if (!amount.trim() || !Number.isFinite(parsedAmount)) {
      setAmountError(t('请输入有效金额'))
      return
    }
    if (mode !== 'override' && (amountValue <= 0 || quotaValue <= 0)) {
      setAmountError(t('请输入大于 0 的金额'))
      return
    }
    if (cycleQuotaManagementEnabled && !reason.trim()) {
      setReasonError(t('必须填写原因'))
      return
    }
    setAmountError(null)
    setReasonError(null)

    setLoading(true)
    try {
      const value =
        mode === 'override' ? parseQuotaFromDollars(amountValue) : quotaValue
      const payload: ManageUserQuotaPayload = {
        id: props.userId,
        action: 'add_quota',
        mode,
        value,
        reason: cycleQuotaManagementEnabled ? reason.trim() : undefined,
      }

      const result = await adjustUserQuota(payload)
      if (result.success) {
        toast.success(t('Quota adjusted successfully'))
        handleOpenChange(false)
        props.onSuccess()
      } else {
        handleServerError(result, t('Failed to adjust quota'))
      }
    } catch (e: unknown) {
      handleServerError(e, t('Failed to adjust quota'))
    } finally {
      setLoading(false)
    }
  }

  const handleCancel = () => {
    handleOpenChange(false)
  }

  const placeholder = tokensOnly
    ? t('Enter amount in tokens')
    : t('Enter amount in {{currency}}', { currency: currencyLabel })
  const modeLabels: Record<QuotaAdjustMode, string> = {
    add: cycleQuotaManagementEnabled ? t('调增') : t('Add'),
    subtract: cycleQuotaManagementEnabled ? t('调减') : t('Subtract'),
    override: t('Override'),
  }
  const submitText: Record<QuotaAdjustMode, string> = {
    add: t('确认调增'),
    subtract: t('确认调减'),
    override: t('确认调整'),
  }

  return (
    <Dialog
      open={props.open}
      onOpenChange={handleOpenChange}
      title={t('Adjust Quota')}
      description={t('手工调额会立即生效')}
      contentHeight='auto'
      bodyClassName='space-y-4'
      footer={
        <>
          <Button variant='outline' onClick={handleCancel}>
            {t('Cancel')}
          </Button>
          <Button onClick={() => void submitAdjustment()} disabled={loading}>
            {loading ? t('Processing...') : submitText[mode]}
          </Button>
        </>
      }
    >
      <div className='space-y-4'>
        <div className='text-muted-foreground text-sm'>{getPreviewText()}</div>

        <div className='space-y-2'>
          <Label id='user-quota-mode-label'>{t('Mode')}</Label>
          <div
            className='flex gap-1'
            role='group'
            aria-labelledby='user-quota-mode-label'
          >
            {(cycleQuotaManagementEnabled
              ? (['add', 'subtract'] as const)
              : (['add', 'subtract', 'override'] as const)
            ).map((m) => (
              <Button
                key={m}
                type='button'
                variant='outline'
                size='sm'
                aria-pressed={mode === m}
                className={cn(
                  mode === m &&
                    'bg-primary text-primary-foreground hover:bg-primary/90 hover:text-primary-foreground'
                )}
                onClick={() => {
                  setMode(m)
                  setAmount('')
                  setAmountError(null)
                }}
              >
                {modeLabels[m]}
              </Button>
            ))}
          </div>
        </div>

        <div className='space-y-2'>
          <Label htmlFor='user-quota-amount'>
            {tokensOnly
              ? t('额度（Token）')
              : `${t('Amount')} (${currencyLabel})`}
          </Label>
          <Input
            id='user-quota-amount'
            type='number'
            step={tokensOnly ? 1 : 0.000001}
            min={mode === 'override' ? undefined : 0}
            aria-invalid={amountError !== null}
            aria-describedby={
              amountError ? 'user-quota-amount-error' : undefined
            }
            placeholder={placeholder}
            value={amount}
            onChange={(e) => {
              setAmount(e.target.value)
              setAmountError(null)
            }}
            onKeyDown={(e) => {
              if (e.key === 'Enter') void submitAdjustment()
            }}
          />
          {amountError && (
            <p
              id='user-quota-amount-error'
              className='text-destructive text-sm'
              role='alert'
            >
              {amountError}
            </p>
          )}
        </div>

        {cycleQuotaManagementEnabled && (
          <div className='space-y-2'>
            <Label htmlFor='user-quota-reason'>{t('Reason')}</Label>
            <Textarea
              id='user-quota-reason'
              value={reason}
              maxLength={255}
              required
              aria-invalid={reasonError !== null}
              aria-describedby={
                reasonError ? 'user-quota-reason-error' : undefined
              }
              onChange={(event) => {
                setReason(event.target.value)
                setReasonError(null)
              }}
            />
            {reasonError && (
              <p
                id='user-quota-reason-error'
                className='text-destructive text-sm'
                role='alert'
              >
                {reasonError}
              </p>
            )}
          </div>
        )}
      </div>
    </Dialog>
  )
}
