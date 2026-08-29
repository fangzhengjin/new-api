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
import { useEffect, useMemo } from 'react'
import { Controller, useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { z } from 'zod'

import { Button } from '@/components/ui/button'
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
  FieldDescription,
  FieldError,
  FieldGroup,
  FieldLabel,
} from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { Spinner } from '@/components/ui/spinner'
import { Switch } from '@/components/ui/switch'
import { getCurrencyDisplay, getCurrencyLabel } from '@/lib/currency'
import { handleServerError } from '@/lib/handle-server-error'

import { createCycle } from '../api'
import type {
  ConcentrationMultiplier,
  CycleDefaults,
  InitialGrantRecommendation,
} from '../types'
import {
  fromShanghaiInput,
  queryKeys,
  quotaFromDisplayAmount,
  quotaToDisplayAmount,
  toLocalInput,
} from '../utils'
import { InitialGrantRecommendationAlert } from './shared'

type FormValues = {
  startAt: string
  endAt: string
  budgetAmount: string
  initialGrantAmount: string
  temporaryQuotaReserveAmount: string
  temporaryQuotaAutoApprovalEnabled: boolean
  temporaryQuotaAutoApprovalSingleAmount: string
  temporaryQuotaAutoApprovalThresholdAmount: string
  temporaryQuotaAutoApprovalMaxCount: string
  temporaryQuotaAutoApprovalMaxAmount: string
  concentrationMultiplier: string
  carryOverBalance: boolean
  carryOverCountsTowardBudget: boolean
}

const currencyAmountPattern = /^\d+(?:\.\d{1,6})?$/
const tokenAmountPattern = /^\d+$/
const concentrationMultiplierPattern = /^(15000|20000|30000)$/

export function CreateCycleDialog(props: {
  open: boolean
  onOpenChange: (open: boolean) => void
  defaults: CycleDefaults
  recommendation: InitialGrantRecommendation | null
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const { meta: currencyMeta } = getCurrencyDisplay()
  const currencyLabel = getCurrencyLabel()
  const tokensOnly = currencyMeta.kind === 'tokens'
  const amountPattern = tokensOnly ? tokenAmountPattern : currencyAmountPattern
  const amountError = tokensOnly
    ? t('Enter quota in tokens')
    : t('Enter quota in {{currency}}', { currency: currencyLabel })
  const schema = useMemo(
    () =>
      z
        .object({
          startAt: z.string().min(1, t('Start time is required')),
          endAt: z.string().min(1, t('End time is required')),
          budgetAmount: z
            .string()
            .regex(amountPattern, amountError)
            .refine(
              (value) => quotaFromDisplayAmount(value) !== null,
              amountError
            ),
          initialGrantAmount: z
            .string()
            .regex(amountPattern, amountError)
            .refine(
              (value) => quotaFromDisplayAmount(value) !== null,
              amountError
            ),
          temporaryQuotaReserveAmount: z
            .string()
            .regex(amountPattern, amountError)
            .refine(
              (value) => quotaFromDisplayAmount(value, true) !== null,
              amountError
            ),
          temporaryQuotaAutoApprovalEnabled: z.boolean(),
          temporaryQuotaAutoApprovalSingleAmount: z.string(),
          temporaryQuotaAutoApprovalThresholdAmount: z.string(),
          temporaryQuotaAutoApprovalMaxCount: z.string(),
          temporaryQuotaAutoApprovalMaxAmount: z.string(),
          concentrationMultiplier: z
            .string()
            .regex(
              concentrationMultiplierPattern,
              t('Select an automatic allocation limit')
            ),
          carryOverBalance: z.boolean(),
          carryOverCountsTowardBudget: z.boolean(),
        })
        .superRefine((values, context) => {
          if (!values.temporaryQuotaAutoApprovalEnabled) return
          const amountFields = [
            [
              'temporaryQuotaAutoApprovalSingleAmount',
              values.temporaryQuotaAutoApprovalSingleAmount,
            ],
            [
              'temporaryQuotaAutoApprovalThresholdAmount',
              values.temporaryQuotaAutoApprovalThresholdAmount,
            ],
            [
              'temporaryQuotaAutoApprovalMaxAmount',
              values.temporaryQuotaAutoApprovalMaxAmount,
            ],
          ] as const
          for (const [path, value] of amountFields) {
            if (
              !amountPattern.test(value) ||
              quotaFromDisplayAmount(value) === null
            ) {
              context.addIssue({
                code: 'custom',
                path: [path],
                message: amountError,
              })
            }
          }
          const maxCount = Number(values.temporaryQuotaAutoApprovalMaxCount)
          if (
            !/^\d+$/.test(values.temporaryQuotaAutoApprovalMaxCount) ||
            !Number.isSafeInteger(maxCount) ||
            maxCount <= 0
          ) {
            context.addIssue({
              code: 'custom',
              path: ['temporaryQuotaAutoApprovalMaxCount'],
              message: t('Enter a positive whole number'),
            })
          }
        }),
    [amountError, amountPattern, t]
  )
  const form = useForm<FormValues>({
    resolver: zodResolver(schema),
    defaultValues: {
      startAt: '',
      endAt: '',
      budgetAmount: '',
      initialGrantAmount: '',
      temporaryQuotaReserveAmount: '0',
      temporaryQuotaAutoApprovalEnabled: false,
      temporaryQuotaAutoApprovalSingleAmount: '0',
      temporaryQuotaAutoApprovalThresholdAmount: '0',
      temporaryQuotaAutoApprovalMaxCount: '0',
      temporaryQuotaAutoApprovalMaxAmount: '0',
      concentrationMultiplier: '15000',
      carryOverBalance: false,
      carryOverCountsTowardBudget: true,
    },
  })

  useEffect(() => {
    if (!props.open) return
    form.reset({
      startAt: toLocalInput(props.defaults.start_at),
      endAt: '',
      budgetAmount: '',
      initialGrantAmount: props.recommendation
        ? quotaToDisplayAmount(props.recommendation.quota)
        : '',
      temporaryQuotaReserveAmount: '0',
      temporaryQuotaAutoApprovalEnabled: false,
      temporaryQuotaAutoApprovalSingleAmount: '0',
      temporaryQuotaAutoApprovalThresholdAmount: '0',
      temporaryQuotaAutoApprovalMaxCount: '0',
      temporaryQuotaAutoApprovalMaxAmount: '0',
      concentrationMultiplier: '15000',
      carryOverBalance: false,
      carryOverCountsTowardBudget: true,
    })
  }, [form, props.defaults, props.open, props.recommendation])

  const mutation = useMutation({
    mutationFn: createCycle,
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: queryKeys.overview }),
        queryClient.invalidateQueries({ queryKey: queryKeys.activities }),
        queryClient.invalidateQueries({ queryKey: queryKeys.cycles }),
        queryClient.invalidateQueries({ queryKey: queryKeys.options }),
      ])
      toast.success(t('Quota cycle created'))
      props.onOpenChange(false)
    },
    onError: handleServerError,
  })

  const submit = form.handleSubmit((values) => {
    const startAt = fromShanghaiInput(values.startAt)
    const endAt = fromShanghaiInput(values.endAt)
    if (endAt <= startAt) {
      form.setError('endAt', {
        message: t('End time must be later than start time'),
      })
      return
    }
    const budgetQuota = quotaFromDisplayAmount(values.budgetAmount)
    const initialGrantQuota = quotaFromDisplayAmount(values.initialGrantAmount)
    const temporaryQuotaReserve = quotaFromDisplayAmount(
      values.temporaryQuotaReserveAmount,
      true
    )
    const temporaryQuotaAutoApprovalSingleQuota =
      values.temporaryQuotaAutoApprovalEnabled
        ? quotaFromDisplayAmount(values.temporaryQuotaAutoApprovalSingleAmount)
        : '0'
    const temporaryQuotaAutoApprovalThresholdQuota =
      values.temporaryQuotaAutoApprovalEnabled
        ? quotaFromDisplayAmount(
            values.temporaryQuotaAutoApprovalThresholdAmount
          )
        : '0'
    const temporaryQuotaAutoApprovalMaxQuota =
      values.temporaryQuotaAutoApprovalEnabled
        ? quotaFromDisplayAmount(values.temporaryQuotaAutoApprovalMaxAmount)
        : '0'
    if (
      !budgetQuota ||
      !initialGrantQuota ||
      temporaryQuotaReserve === null ||
      temporaryQuotaAutoApprovalSingleQuota === null ||
      temporaryQuotaAutoApprovalThresholdQuota === null ||
      temporaryQuotaAutoApprovalMaxQuota === null
    ) {
      return
    }
    mutation.mutate({
      cycle_start_at: startAt,
      cycle_end_at: endAt,
      budget_quota: budgetQuota,
      initial_grant_quota: initialGrantQuota,
      temporary_quota_reserve: temporaryQuotaReserve,
      temporary_quota_auto_approval_enabled:
        values.temporaryQuotaAutoApprovalEnabled,
      temporary_quota_auto_approval_single_quota:
        temporaryQuotaAutoApprovalSingleQuota,
      temporary_quota_auto_approval_threshold_quota:
        temporaryQuotaAutoApprovalThresholdQuota,
      temporary_quota_auto_approval_max_count:
        values.temporaryQuotaAutoApprovalEnabled
          ? Number(values.temporaryQuotaAutoApprovalMaxCount)
          : 0,
      temporary_quota_auto_approval_max_quota:
        temporaryQuotaAutoApprovalMaxQuota,
      concentration_multiplier_basis_points: Number(
        values.concentrationMultiplier
      ) as ConcentrationMultiplier,
      balance_policy: values.carryOverBalance ? 'carry' : 'reset',
      carry_over_counts_toward_budget:
        values.carryOverBalance && values.carryOverCountsTowardBudget,
    })
  })

  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      <DialogContent className='max-h-[90vh] overflow-y-auto sm:max-w-2xl'>
        <DialogHeader>
          <DialogTitle>{t('Create quota cycle')}</DialogTitle>
          <DialogDescription>
            {t(
              'Define the valid time range and cycle total quota. Allocation is handled through plans after the cycle starts'
            )}
          </DialogDescription>
        </DialogHeader>
        <form id='create-quota-cycle' onSubmit={submit}>
          <FieldGroup>
            {props.recommendation && (
              <InitialGrantRecommendationAlert
                recommendation={props.recommendation}
                t={t}
              />
            )}
            <div className='grid gap-4 sm:grid-cols-2'>
              <Field data-invalid={!!form.formState.errors.startAt}>
                <FieldLabel htmlFor='quota-cycle-start'>
                  {t('Start time')}
                </FieldLabel>
                <Input
                  id='quota-cycle-start'
                  aria-invalid={!!form.formState.errors.startAt}
                  aria-describedby={
                    form.formState.errors.startAt
                      ? 'quota-cycle-start-error'
                      : undefined
                  }
                  type='datetime-local'
                  {...form.register('startAt')}
                />
                <FieldError
                  id='quota-cycle-start-error'
                  errors={[form.formState.errors.startAt]}
                />
              </Field>
              <Field data-invalid={!!form.formState.errors.endAt}>
                <FieldLabel htmlFor='quota-cycle-end'>
                  {t('End time')}
                </FieldLabel>
                <Input
                  id='quota-cycle-end'
                  aria-invalid={!!form.formState.errors.endAt}
                  aria-describedby={
                    form.formState.errors.endAt
                      ? 'quota-cycle-end-error'
                      : undefined
                  }
                  type='datetime-local'
                  {...form.register('endAt')}
                />
                <FieldError
                  id='quota-cycle-end-error'
                  errors={[form.formState.errors.endAt]}
                />
              </Field>
            </div>
            <div className='grid gap-4 sm:grid-cols-2'>
              <Field data-invalid={!!form.formState.errors.budgetAmount}>
                <FieldLabel htmlFor='quota-cycle-budget'>
                  {t('Budget')} ({currencyLabel})
                </FieldLabel>
                <Input
                  id='quota-cycle-budget'
                  aria-invalid={!!form.formState.errors.budgetAmount}
                  aria-describedby={
                    form.formState.errors.budgetAmount
                      ? 'quota-cycle-budget-error'
                      : undefined
                  }
                  inputMode='decimal'
                  placeholder='10000.00'
                  {...form.register('budgetAmount')}
                />
                <FieldError
                  id='quota-cycle-budget-error'
                  errors={[form.formState.errors.budgetAmount]}
                />
              </Field>
              <Field data-invalid={!!form.formState.errors.initialGrantAmount}>
                <FieldLabel htmlFor='quota-cycle-grant'>
                  {t('Initial grant per user')} ({currencyLabel})
                </FieldLabel>
                <Input
                  id='quota-cycle-grant'
                  aria-invalid={!!form.formState.errors.initialGrantAmount}
                  aria-describedby={
                    form.formState.errors.initialGrantAmount
                      ? 'quota-cycle-grant-error'
                      : undefined
                  }
                  inputMode='decimal'
                  placeholder='100.00'
                  {...form.register('initialGrantAmount')}
                />
                <FieldError
                  id='quota-cycle-grant-error'
                  errors={[form.formState.errors.initialGrantAmount]}
                />
              </Field>
              <Field
                data-invalid={!!form.formState.errors.concentrationMultiplier}
              >
                <FieldLabel htmlFor='quota-cycle-concentration-multiplier'>
                  {t('Automatic allocation limit')}
                </FieldLabel>
                <NativeSelect
                  id='quota-cycle-concentration-multiplier'
                  className='w-full'
                  aria-invalid={!!form.formState.errors.concentrationMultiplier}
                  aria-describedby={
                    form.formState.errors.concentrationMultiplier
                      ? 'quota-cycle-concentration-multiplier-description quota-cycle-concentration-multiplier-error'
                      : 'quota-cycle-concentration-multiplier-description'
                  }
                  {...form.register('concentrationMultiplier')}
                >
                  <NativeSelectOption value='15000'>1.5×</NativeSelectOption>
                  <NativeSelectOption value='20000'>2×</NativeSelectOption>
                  <NativeSelectOption value='30000'>3×</NativeSelectOption>
                </NativeSelect>
                <FieldDescription id='quota-cycle-concentration-multiplier-description'>
                  {t(
                    "The system compares each user's period spend plus current balance with the per-user share for the ordinary allocation pool, 1.5× is strictest and 3× allows the most, initial grants, manual grants, and temporary quota are not limited"
                  )}
                </FieldDescription>
                <FieldError
                  id='quota-cycle-concentration-multiplier-error'
                  errors={[form.formState.errors.concentrationMultiplier]}
                />
              </Field>
              <Field
                data-invalid={
                  !!form.formState.errors.temporaryQuotaReserveAmount
                }
              >
                <FieldLabel htmlFor='quota-cycle-temporary-reserve'>
                  {t('Temporary quota reserve')} ({currencyLabel})
                </FieldLabel>
                <Input
                  id='quota-cycle-temporary-reserve'
                  aria-invalid={
                    !!form.formState.errors.temporaryQuotaReserveAmount
                  }
                  aria-describedby={
                    form.formState.errors.temporaryQuotaReserveAmount
                      ? 'quota-cycle-temporary-reserve-description quota-cycle-temporary-reserve-error'
                      : 'quota-cycle-temporary-reserve-description'
                  }
                  inputMode='decimal'
                  placeholder='0'
                  {...form.register('temporaryQuotaReserveAmount')}
                />
                <FieldError
                  id='quota-cycle-temporary-reserve-error'
                  errors={[form.formState.errors.temporaryQuotaReserveAmount]}
                />
                <FieldDescription id='quota-cycle-temporary-reserve-description'>
                  {t(
                    'Reserved from the cycle quota for temporary quota requests'
                  )}
                </FieldDescription>
              </Field>
            </div>
            <Controller
              name='temporaryQuotaAutoApprovalEnabled'
              control={form.control}
              render={({ field }) => (
                <Field orientation='horizontal'>
                  <div className='flex-1'>
                    <FieldLabel htmlFor='quota-cycle-auto-temporary-quota'>
                      {t(
                        'Automatically approve eligible temporary quota requests'
                      )}
                    </FieldLabel>
                    <FieldDescription>
                      {t(
                        'Disabled by default, configure every limit before activation because the policy is frozen after the cycle starts'
                      )}
                    </FieldDescription>
                  </div>
                  <Switch
                    id='quota-cycle-auto-temporary-quota'
                    checked={field.value}
                    onCheckedChange={field.onChange}
                  />
                </Field>
              )}
            />
            {form.watch('temporaryQuotaAutoApprovalEnabled') && (
              <div className='grid gap-4 sm:grid-cols-2'>
                <Field
                  data-invalid={
                    !!form.formState.errors
                      .temporaryQuotaAutoApprovalSingleAmount
                  }
                >
                  <FieldLabel htmlFor='quota-cycle-auto-single'>
                    {t('Automatic single-request limit')} ({currencyLabel})
                  </FieldLabel>
                  <Input
                    id='quota-cycle-auto-single'
                    aria-invalid={
                      !!form.formState.errors
                        .temporaryQuotaAutoApprovalSingleAmount
                    }
                    aria-describedby={
                      form.formState.errors
                        .temporaryQuotaAutoApprovalSingleAmount
                        ? 'quota-cycle-auto-single-error'
                        : undefined
                    }
                    inputMode='decimal'
                    {...form.register('temporaryQuotaAutoApprovalSingleAmount')}
                  />
                  <FieldError
                    id='quota-cycle-auto-single-error'
                    errors={[
                      form.formState.errors
                        .temporaryQuotaAutoApprovalSingleAmount,
                    ]}
                  />
                </Field>
                <Field
                  data-invalid={
                    !!form.formState.errors
                      .temporaryQuotaAutoApprovalThresholdAmount
                  }
                >
                  <FieldLabel htmlFor='quota-cycle-auto-threshold'>
                    {t('Automatic balance threshold')} ({currencyLabel})
                  </FieldLabel>
                  <Input
                    id='quota-cycle-auto-threshold'
                    aria-invalid={
                      !!form.formState.errors
                        .temporaryQuotaAutoApprovalThresholdAmount
                    }
                    aria-describedby={
                      form.formState.errors
                        .temporaryQuotaAutoApprovalThresholdAmount
                        ? 'quota-cycle-auto-threshold-error'
                        : undefined
                    }
                    inputMode='decimal'
                    {...form.register(
                      'temporaryQuotaAutoApprovalThresholdAmount'
                    )}
                  />
                  <FieldError
                    id='quota-cycle-auto-threshold-error'
                    errors={[
                      form.formState.errors
                        .temporaryQuotaAutoApprovalThresholdAmount,
                    ]}
                  />
                </Field>
                <Field
                  data-invalid={
                    !!form.formState.errors.temporaryQuotaAutoApprovalMaxCount
                  }
                >
                  <FieldLabel htmlFor='quota-cycle-auto-count'>
                    {t('Automatic requests per user')}
                  </FieldLabel>
                  <Input
                    id='quota-cycle-auto-count'
                    aria-invalid={
                      !!form.formState.errors.temporaryQuotaAutoApprovalMaxCount
                    }
                    aria-describedby={
                      form.formState.errors.temporaryQuotaAutoApprovalMaxCount
                        ? 'quota-cycle-auto-count-error'
                        : undefined
                    }
                    type='number'
                    min={1}
                    step={1}
                    {...form.register('temporaryQuotaAutoApprovalMaxCount')}
                  />
                  <FieldError
                    id='quota-cycle-auto-count-error'
                    errors={[
                      form.formState.errors.temporaryQuotaAutoApprovalMaxCount,
                    ]}
                  />
                </Field>
                <Field
                  data-invalid={
                    !!form.formState.errors.temporaryQuotaAutoApprovalMaxAmount
                  }
                >
                  <FieldLabel htmlFor='quota-cycle-auto-total'>
                    {t('Automatic total per user')} ({currencyLabel})
                  </FieldLabel>
                  <Input
                    id='quota-cycle-auto-total'
                    aria-invalid={
                      !!form.formState.errors
                        .temporaryQuotaAutoApprovalMaxAmount
                    }
                    aria-describedby={
                      form.formState.errors.temporaryQuotaAutoApprovalMaxAmount
                        ? 'quota-cycle-auto-total-error'
                        : undefined
                    }
                    inputMode='decimal'
                    {...form.register('temporaryQuotaAutoApprovalMaxAmount')}
                  />
                  <FieldError
                    id='quota-cycle-auto-total-error'
                    errors={[
                      form.formState.errors.temporaryQuotaAutoApprovalMaxAmount,
                    ]}
                  />
                </Field>
              </div>
            )}
            <Controller
              name='carryOverBalance'
              control={form.control}
              render={({ field }) => (
                <Field orientation='horizontal'>
                  <div className='flex-1'>
                    <FieldLabel htmlFor='quota-cycle-carry-over'>
                      {t('Carry over balance')}
                    </FieldLabel>
                    <FieldDescription>
                      {t('Keep managed user balances when this cycle closes')}
                    </FieldDescription>
                  </div>
                  <Switch
                    id='quota-cycle-carry-over'
                    checked={field.value}
                    onCheckedChange={field.onChange}
                  />
                </Field>
              )}
            />
            {form.watch('carryOverBalance') && (
              <Controller
                name='carryOverCountsTowardBudget'
                control={form.control}
                render={({ field }) => (
                  <Field
                    orientation='horizontal'
                    className='ml-4 border-l pl-4'
                  >
                    <div className='flex-1'>
                      <FieldLabel htmlFor='quota-cycle-carry-over-budget'>
                        {t('Count toward new cycle quota')}
                      </FieldLabel>
                      <FieldDescription>
                        {t(
                          "Include carried balances in the next cycle's allocated quota"
                        )}
                      </FieldDescription>
                    </div>
                    <Switch
                      id='quota-cycle-carry-over-budget'
                      checked={field.value}
                      onCheckedChange={field.onChange}
                    />
                  </Field>
                )}
              />
            )}
          </FieldGroup>
        </form>
        <DialogFooter>
          <Button
            type='button'
            variant='outline'
            disabled={mutation.isPending}
            onClick={() => props.onOpenChange(false)}
          >
            {t('Cancel')}
          </Button>
          <Button
            type='submit'
            form='create-quota-cycle'
            disabled={mutation.isPending}
          >
            {mutation.isPending && <Spinner data-icon='inline-start' />}
            {t('Create')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
