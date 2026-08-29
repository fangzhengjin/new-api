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
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useMemo, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import { Combobox } from '@/components/ui/combobox'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  Field,
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
} from '../../quota-management/utils'
import { submitSelfTemporaryQuota, temporaryQuotaQueryKeys } from '../api'
import {
  createTemporaryQuotaFormSchema,
  type TemporaryQuotaFormValues,
} from '../lib/temporary-quota-form'
import type { TemporaryQuotaOverview } from '../types'

function createTemporaryQuotaIdempotencyKey() {
  if (typeof globalThis.crypto.randomUUID === 'function') {
    return globalThis.crypto.randomUUID()
  }
  return [...globalThis.crypto.getRandomValues(new Uint8Array(16))]
    .map((byte) => byte.toString(16).padStart(2, '0'))
    .join('')
}

export function TemporaryQuotaDialog(props: {
  open: boolean
  overview?: TemporaryQuotaOverview
  onOpenChange: (open: boolean) => void
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [idempotencyKey, setIdempotencyKey] = useState(
    createTemporaryQuotaIdempotencyKey
  )
  const { meta } = getCurrencyDisplay()
  const currencyLabel = getCurrencyLabel()
  const amountPattern = useMemo(
    () => (meta.kind === 'tokens' ? /^\d+$/ : /^\d+(?:\.\d{1,6})?$/),
    [meta.kind]
  )
  const amountError =
    meta.kind === 'tokens'
      ? t('Enter quota in tokens')
      : t('Enter quota in {{currency}}', { currency: currencyLabel })
  const schema = useMemo(
    () =>
      createTemporaryQuotaFormSchema({
        amountPattern,
        amountError,
        amountIsValid: (value) => quotaFromDisplayAmount(value) !== null,
        projectError: t('Project is required'),
        reasonRequiredError: t('Reason is required'),
        reasonLengthError: t('Reason must be 500 characters or fewer'),
      }),
    [amountError, amountPattern, t]
  )
  const form = useForm<TemporaryQuotaFormValues>({
    resolver: zodResolver(schema),
    defaultValues: { amount: '', project: '', reason: '' },
  })

  const close = () => {
    form.reset()
    setIdempotencyKey(createTemporaryQuotaIdempotencyKey())
    props.onOpenChange(false)
  }
  const mutation = useMutation({
    mutationFn: submitSelfTemporaryQuota,
    onSuccess: async (result) => {
      await queryClient.invalidateQueries({
        queryKey: temporaryQuotaQueryKeys.all,
      })
      if (result.request.status === 'executed') {
        toast.success(t('Temporary quota granted'))
      } else if (result.request.status === 'pending') {
        toast.success(t('Temporary quota request sent for review'))
      } else {
        toast.error(result.request.review_reason)
      }
      close()
    },
    onError: handleServerError,
  })
  const submit = form.handleSubmit((values) => {
    const requestedQuota = quotaFromDisplayAmount(values.amount)
    if (!requestedQuota) return
    mutation.mutate({
      idempotency_key: idempotencyKey,
      requested_quota: requestedQuota,
      project: values.project,
      reason: values.reason.trim(),
    })
  })

  return (
    <Dialog
      open={props.open}
      onOpenChange={(open) => {
        if (!open && !mutation.isPending) close()
      }}
    >
      <DialogContent className='sm:max-w-lg'>
        <form onSubmit={submit} className='contents'>
          <DialogHeader>
            <DialogTitle>{t('Request temporary quota')}</DialogTitle>
            <DialogDescription>
              {t(
                'When your balance is insufficient and affects your work, submit a temporary quota request for the current cycle'
              )}
            </DialogDescription>
          </DialogHeader>

          <div className='bg-muted/40 rounded-xl border px-4 py-3'>
            <div className='text-muted-foreground text-sm'>
              {t('Current account balance')}
            </div>
            <div className='mt-1 text-xl font-semibold tabular-nums'>
              {quotaToDisplayAmount(props.overview?.current_quota ?? '0')}{' '}
              <span className='text-muted-foreground text-sm font-normal'>
                {currencyLabel}
              </span>
            </div>
          </div>

          <FieldGroup>
            <Field data-invalid={!!form.formState.errors.project}>
              <FieldLabel htmlFor='temporary-quota-project'>
                {t('Project')}
              </FieldLabel>
              <Combobox
                id='temporary-quota-project'
                aria-invalid={!!form.formState.errors.project}
                aria-describedby={
                  form.formState.errors.project
                    ? 'temporary-quota-project-error'
                    : undefined
                }
                options={(props.overview?.projects ?? []).map((project) => ({
                  value: project,
                  label: project,
                }))}
                value={form.watch('project')}
                onValueChange={(value) =>
                  form.setValue('project', value ?? '', {
                    shouldDirty: true,
                    shouldValidate: true,
                  })
                }
                placeholder={t('Select a project')}
                searchPlaceholder={t('Search projects')}
                emptyText={t('No matching projects')}
                allowCustomValue={false}
                openOnFocus
                disabled={mutation.isPending}
              />
              <FieldError
                id='temporary-quota-project-error'
                errors={[form.formState.errors.project]}
              />
            </Field>
            <Field data-invalid={!!form.formState.errors.amount}>
              <FieldLabel htmlFor='temporary-quota-amount'>
                {t('Requested quota')} ({currencyLabel})
              </FieldLabel>
              <Input
                id='temporary-quota-amount'
                aria-invalid={!!form.formState.errors.amount}
                aria-describedby={
                  form.formState.errors.amount
                    ? 'temporary-quota-amount-error'
                    : undefined
                }
                inputMode={meta.kind === 'tokens' ? 'numeric' : 'decimal'}
                disabled={mutation.isPending}
                {...form.register('amount')}
              />
              <FieldError
                id='temporary-quota-amount-error'
                errors={[form.formState.errors.amount]}
              />
            </Field>
            <Field data-invalid={!!form.formState.errors.reason}>
              <FieldLabel htmlFor='temporary-quota-reason'>
                {t('Reason')}
              </FieldLabel>
              <Textarea
                id='temporary-quota-reason'
                aria-invalid={!!form.formState.errors.reason}
                aria-describedby={
                  form.formState.errors.reason
                    ? 'temporary-quota-reason-error'
                    : undefined
                }
                disabled={mutation.isPending}
                {...form.register('reason')}
              />
              <FieldError
                id='temporary-quota-reason-error'
                errors={[form.formState.errors.reason]}
              />
            </Field>
          </FieldGroup>

          <DialogFooter>
            <Button
              type='button'
              variant='outline'
              onClick={close}
              disabled={mutation.isPending}
            >
              {t('Cancel')}
            </Button>
            <Button type='submit' disabled={mutation.isPending}>
              {mutation.isPending && <Spinner data-icon='inline-start' />}
              {t('Submit')}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
