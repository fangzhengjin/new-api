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
import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useRef } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { z } from 'zod'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import {
  Field,
  FieldDescription,
  FieldError,
  FieldGroup,
  FieldLabel,
} from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Spinner } from '@/components/ui/spinner'
import { Textarea } from '@/components/ui/textarea'
import { getCurrencyDisplay, getCurrencyLabel } from '@/lib/currency'
import { handleServerError } from '@/lib/handle-server-error'

import {
  quotaFromDisplayAmount,
  quotaToDisplayAmount,
} from '../quota-management/utils'
import { getSelfQuotaRecovery, submitSelfQuotaRecovery } from './api'

type FormValues = {
  amount: string
  reason: string
}

const recoveryQueryKey = ['quota-recovery', 'self'] as const

export function QuotaRecoveryCard() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const idempotencyKey = useRef(crypto.randomUUID())
  const { meta } = getCurrencyDisplay()
  const currencyLabel = getCurrencyLabel()
  const amountPattern = meta.kind === 'tokens' ? /^\d+$/ : /^\d+(?:\.\d{1,6})?$/
  const amountError =
    meta.kind === 'tokens'
      ? t('Enter quota in tokens')
      : t('Enter quota in {{currency}}', { currency: currencyLabel })
  const schema = z.object({
    amount: z
      .string()
      .regex(amountPattern, amountError)
      .refine((value) => quotaFromDisplayAmount(value) !== null, amountError),
    reason: z
      .string()
      .trim()
      .min(1, t('Reason is required'))
      .max(500, t('Reason must be 500 characters or fewer')),
  })
  const form = useForm<FormValues>({
    resolver: zodResolver(schema),
    defaultValues: { amount: '', reason: '' },
  })
  const query = useQuery({
    queryKey: recoveryQueryKey,
    queryFn: getSelfQuotaRecovery,
  })
  const mutation = useMutation({
    mutationFn: submitSelfQuotaRecovery,
    onSuccess: async (result) => {
      await queryClient.invalidateQueries({ queryKey: recoveryQueryKey })
      if (result.request.status === 'executed') {
        toast.success(t('Temporary quota granted'))
      } else if (result.request.status === 'pending') {
        toast.success(t('Recovery request sent for manual review'))
      } else {
        toast.error(
          result.request.review_reason || t('Recovery request rejected')
        )
      }
      form.reset()
      idempotencyKey.current = crypto.randomUUID()
    },
    onError: handleServerError,
  })
  const submit = form.handleSubmit((values) => {
    const requestedQuota = quotaFromDisplayAmount(values.amount)
    if (!requestedQuota) return
    mutation.mutate({
      idempotency_key: idempotencyKey.current,
      requested_quota: requestedQuota,
      reason: values.reason.trim(),
    })
  })

  if (query.isPending) {
    return (
      <Card data-card-hover='false'>
        <CardContent className='flex items-center gap-2 py-6'>
          <Spinner /> {t('Loading recovery policy...')}
        </CardContent>
      </Card>
    )
  }
  if (query.isError) {
    return (
      <Card data-card-hover='false'>
        <CardHeader>
          <CardTitle>{t('Temporary quota recovery')}</CardTitle>
          <CardDescription>{query.error.message}</CardDescription>
        </CardHeader>
      </Card>
    )
  }

  const overview = query.data
  return (
    <Card data-card-hover='false'>
      <CardHeader>
        <div className='flex items-start justify-between gap-3'>
          <div>
            <CardTitle>{t('Temporary quota recovery')}</CardTitle>
            <CardDescription>
              {t('Request continuity quota when your managed balance is low.')}
            </CardDescription>
          </div>
          <Badge variant={overview.reserve_available ? 'outline' : 'secondary'}>
            {overview.reserve_available
              ? t('Recovery available')
              : t('Recovery pool exhausted')}
          </Badge>
        </div>
      </CardHeader>
      <CardContent className='space-y-5'>
        <div className='text-muted-foreground grid gap-1 text-sm'>
          <div>
            {t('Current balance')}:{' '}
            {quotaToDisplayAmount(overview.current_quota)} {currencyLabel}
          </div>
          <div>
            {overview.auto_recovery_enabled
              ? t(
                  'Small eligible requests may be approved automatically; other requests go to an administrator.'
                )
              : t(
                  'Automatic approval is disabled; requests go to an administrator.'
                )}
          </div>
        </div>
        <form onSubmit={submit}>
          <FieldGroup>
            <Field data-invalid={!!form.formState.errors.amount}>
              <FieldLabel htmlFor='quota-recovery-amount'>
                {t('Requested quota')} ({currencyLabel})
              </FieldLabel>
              <Input
                id='quota-recovery-amount'
                inputMode='decimal'
                disabled={!overview.reserve_available || mutation.isPending}
                {...form.register('amount')}
              />
              {overview.auto_recovery_enabled && (
                <FieldDescription>
                  {t('Automatic single-request limit')}:{' '}
                  {quotaToDisplayAmount(overview.auto_recovery_single_quota)}{' '}
                  {currencyLabel}
                </FieldDescription>
              )}
              <FieldError errors={[form.formState.errors.amount]} />
            </Field>
            <Field data-invalid={!!form.formState.errors.reason}>
              <FieldLabel htmlFor='quota-recovery-reason'>
                {t('Reason')}
              </FieldLabel>
              <Textarea
                id='quota-recovery-reason'
                maxLength={500}
                disabled={!overview.reserve_available || mutation.isPending}
                {...form.register('reason')}
              />
              <FieldError errors={[form.formState.errors.reason]} />
            </Field>
            <Button
              type='submit'
              disabled={!overview.reserve_available || mutation.isPending}
            >
              {mutation.isPending && <Spinner data-icon='inline-start' />}
              {t('Request temporary quota')}
            </Button>
          </FieldGroup>
        </form>
        {overview.requests.length > 0 && (
          <div className='space-y-2' aria-label={t('Recent recovery requests')}>
            <div className='text-sm font-medium'>
              {t('Recent recovery requests')}
            </div>
            {overview.requests.slice(0, 3).map((request) => {
              let statusLabel = t('Rejected')
              if (request.status === 'pending') {
                statusLabel = t('Pending review')
              }
              if (request.status === 'executed') statusLabel = t('Granted')
              return (
                <div key={request.id} className='rounded-lg border p-3 text-sm'>
                  <div className='flex items-center justify-between gap-2'>
                    <span>
                      {quotaToDisplayAmount(request.requested_quota)}{' '}
                      {currencyLabel}
                    </span>
                    <Badge variant='outline'>{statusLabel}</Badge>
                  </div>
                  <div className='text-muted-foreground mt-1'>
                    {request.reason}
                  </div>
                  {request.review_reason && (
                    <div className='mt-1'>{request.review_reason}</div>
                  )}
                </div>
              )
            })}
          </div>
        )}
      </CardContent>
    </Card>
  )
}
