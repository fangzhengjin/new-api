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

import { ConfirmDialog } from '@/components/confirm-dialog'
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
import type {
  ManageUserQuotaPayload,
  ManualQuotaAdjustmentData,
  QuotaAdjustMode,
} from '../types'

interface UserQuotaDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  userId: number
  currentQuota: number
  onSuccess: () => void
}

function formatServerQuota(value?: string): string {
  if (!value) return ''
  const numericValue = Number(value)
  return Number.isSafeInteger(numericValue) ? formatQuota(numericValue) : value
}

export function UserQuotaDialog(props: UserQuotaDialogProps) {
  const { t } = useTranslation()
  const [mode, setMode] = useState<QuotaAdjustMode>('add')
  const [amount, setAmount] = useState('')
  const [reason, setReason] = useState('')
  const [stageOverageDetails, setStageOverageDetails] =
    useState<ManualQuotaAdjustmentData | null>(null)
  const [amountError, setAmountError] = useState<string | null>(null)
  const [loading, setLoading] = useState(false)
  const companyQuotaModeEnabled = useSystemConfigStore(
    (state) => state.config.companyQuotaModeEnabled === true
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
      setStageOverageDetails(null)
      setAmountError(null)
    }
    props.onOpenChange(open)
  }

  const submitAdjustment = async (confirmed: boolean) => {
    if (!amount.trim() || !Number.isFinite(parsedAmount)) {
      setAmountError(t('Enter a valid amount'))
      return
    }
    if (mode !== 'override' && (amountValue <= 0 || quotaValue <= 0)) {
      setAmountError(t('Enter an amount greater than 0'))
      return
    }
    setAmountError(null)

    setLoading(true)
    try {
      const value =
        mode === 'override' ? parseQuotaFromDollars(amountValue) : quotaValue
      const payload: ManageUserQuotaPayload = {
        id: props.userId,
        action: 'add_quota',
        mode,
        value,
        reason: companyQuotaModeEnabled ? reason.trim() : undefined,
      }
      if (companyQuotaModeEnabled) payload.confirmed = confirmed

      const result = await adjustUserQuota(payload)
      const stageOverage = result.data?.stage_overage_quota
      if (
        !result.success &&
        stageOverage &&
        result.data?.stage_cap_quota &&
        result.data.occupied_after_quota
      ) {
        setStageOverageDetails(result.data)
        return
      }
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
    add: companyQuotaModeEnabled ? t('Increase') : t('Add'),
    subtract: companyQuotaModeEnabled ? t('Decrease') : t('Subtract'),
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
          <Button
            onClick={() => void submitAdjustment(false)}
            disabled={loading}
          >
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
            {(companyQuotaModeEnabled
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
                  setStageOverageDetails(null)
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
              setStageOverageDetails(null)
              setAmountError(null)
            }}
            onKeyDown={(e) => {
              if (e.key === 'Enter') void submitAdjustment(false)
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

        {companyQuotaModeEnabled && (
          <div className='space-y-2'>
            <Label htmlFor='user-quota-reason'>{t('Reason (optional)')}</Label>
            <Textarea
              id='user-quota-reason'
              value={reason}
              maxLength={255}
              onChange={(event) => {
                setReason(event.target.value)
                setStageOverageDetails(null)
              }}
            />
          </div>
        )}
      </div>
      <ConfirmDialog
        open={stageOverageDetails !== null}
        onOpenChange={(open) => {
          if (!open) setStageOverageDetails(null)
        }}
        title={t('This increase exceeds the current stage company quota limit')}
        desc={
          stageOverageDetails ? (
            <div className='space-y-3'>
              <dl className='space-y-2'>
                <div className='flex justify-between gap-4'>
                  <dt>{t('Current stage company quota limit')}</dt>
                  <dd className='text-foreground font-medium'>
                    {formatServerQuota(stageOverageDetails.stage_cap_quota)}
                  </dd>
                </div>
                <div className='flex justify-between gap-4'>
                  <dt>
                    {t(
                      'Cycle consumption plus all managed account balances after increase'
                    )}
                  </dt>
                  <dd className='text-foreground font-medium'>
                    {formatServerQuota(
                      stageOverageDetails.occupied_after_quota
                    )}
                  </dd>
                </div>
                <div className='flex justify-between gap-4'>
                  <dt>{t('Amount over the limit')}</dt>
                  <dd className='text-foreground font-medium'>
                    {formatServerQuota(stageOverageDetails.stage_overage_quota)}
                  </dd>
                </div>
              </dl>
              <p>{t('The manual increase takes effect immediately.')}</p>
            </div>
          ) : (
            ''
          )
        }
        confirmText={
          loading
            ? t('Processing...')
            : t('Increase anyway by {{amount}}', {
                amount: formatQuota(quotaValue),
              })
        }
        handleConfirm={() => void submitAdjustment(true)}
        isLoading={loading}
      />
    </Dialog>
  )
}
