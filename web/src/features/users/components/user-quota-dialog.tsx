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
  const quotaValue = parseQuotaFromDollars(amountValue)

  const getPreviewText = () => {
    const current = props.currentQuota
    const val = quotaValue
    if (
      !amount.trim() ||
      !Number.isFinite(parsedAmount) ||
      (mode !== 'override' && (amountValue <= 0 || quotaValue <= 0))
    ) {
      return t('Current balance: {{current}}', {
        current: formatQuota(current),
      })
    }
    switch (mode) {
      case 'add':
        return t(
          'Current balance: {{current}} → Balance after increase: {{after}} (+{{amount}})',
          {
            current: formatQuota(current),
            after: formatQuota(current + val),
            amount: formatQuota(val),
          }
        )
      case 'subtract':
        return t(
          'Current balance: {{current}} → Balance after decrease: {{after}} (-{{amount}})',
          {
            current: formatQuota(current),
            after: formatQuota(current - val),
            amount: formatQuota(val),
          }
        )
      case 'override': {
        const overrideQuota = parseQuotaFromDollars(amountValue)
        return t(
          'Current balance: {{current}} → Balance after adjustment: {{after}}',
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
      setAmountError(t('Enter a valid amount'))
      return
    }
    if (mode !== 'override' && (amountValue <= 0 || quotaValue <= 0)) {
      setAmountError(t('Enter an amount greater than 0'))
      return
    }
    if (cycleQuotaManagementEnabled && !reason.trim()) {
      setReasonError(t('Reason is required'))
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
        toast.error(result.message || t('Failed to adjust quota'))
      }
    } catch (e: unknown) {
      toast.error(e instanceof Error ? e.message : t('Failed to adjust quota'))
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
    add: cycleQuotaManagementEnabled ? t('Increase') : t('Add'),
    subtract: cycleQuotaManagementEnabled ? t('Decrease') : t('Subtract'),
    override: t('Override'),
  }
  const submitText: Record<QuotaAdjustMode, string> = {
    add: t('Confirm increase'),
    subtract: t('Confirm decrease'),
    override: t('Confirm adjustment'),
  }

  return (
    <Dialog
      open={props.open}
      onOpenChange={handleOpenChange}
      title={t('Adjust Quota')}
      description={t('Manual quota adjustments take effect immediately.')}
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
              ? t('Quota amount (tokens)')
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
