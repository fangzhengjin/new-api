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
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Spinner } from '@/components/ui/spinner'
import { Switch } from '@/components/ui/switch'
import { getCurrencyDisplay, getCurrencyLabel } from '@/lib/currency'
import { handleServerError } from '@/lib/handle-server-error'

import { createCycle } from '../api'
import type {
  BalancePolicy,
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
  recoveryReserveAmount: string
  autoRecoveryEnabled: boolean
  autoRecoverySingleAmount: string
  autoRecoveryThresholdAmount: string
  autoRecoveryMaxCount: string
  autoRecoveryMaxAmount: string
  balancePolicy: BalancePolicy
}

const currencyAmountPattern = /^\d+(?:\.\d{1,6})?$/
const tokenAmountPattern = /^\d+$/

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
          recoveryReserveAmount: z
            .string()
            .regex(amountPattern, amountError)
            .refine(
              (value) => quotaFromDisplayAmount(value, true) !== null,
              amountError
            ),
          autoRecoveryEnabled: z.boolean(),
          autoRecoverySingleAmount: z.string(),
          autoRecoveryThresholdAmount: z.string(),
          autoRecoveryMaxCount: z.string(),
          autoRecoveryMaxAmount: z.string(),
          balancePolicy: z.enum(['reset', 'carry']),
        })
        .superRefine((values, context) => {
          if (!values.autoRecoveryEnabled) return
          const amountFields = [
            ['autoRecoverySingleAmount', values.autoRecoverySingleAmount],
            ['autoRecoveryThresholdAmount', values.autoRecoveryThresholdAmount],
            ['autoRecoveryMaxAmount', values.autoRecoveryMaxAmount],
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
          const maxCount = Number(values.autoRecoveryMaxCount)
          if (
            !/^\d+$/.test(values.autoRecoveryMaxCount) ||
            !Number.isSafeInteger(maxCount) ||
            maxCount <= 0
          ) {
            context.addIssue({
              code: 'custom',
              path: ['autoRecoveryMaxCount'],
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
      recoveryReserveAmount: '0',
      autoRecoveryEnabled: false,
      autoRecoverySingleAmount: '0',
      autoRecoveryThresholdAmount: '0',
      autoRecoveryMaxCount: '0',
      autoRecoveryMaxAmount: '0',
      balancePolicy: 'reset',
    },
  })

  useEffect(() => {
    if (!props.open) return
    form.reset({
      startAt: toLocalInput(props.defaults.start_at),
      endAt: toLocalInput(props.defaults.end_at),
      budgetAmount: '',
      initialGrantAmount: props.recommendation
        ? quotaToDisplayAmount(props.recommendation.quota)
        : '',
      recoveryReserveAmount: '0',
      autoRecoveryEnabled: false,
      autoRecoverySingleAmount: '0',
      autoRecoveryThresholdAmount: '0',
      autoRecoveryMaxCount: '0',
      autoRecoveryMaxAmount: '0',
      balancePolicy: 'reset',
    })
  }, [form, props.defaults, props.open, props.recommendation])

  const mutation = useMutation({
    mutationFn: createCycle,
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: queryKeys.cycles })
      await queryClient.invalidateQueries({ queryKey: queryKeys.options })
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
    const recoveryReserveQuota = quotaFromDisplayAmount(
      values.recoveryReserveAmount,
      true
    )
    const autoRecoverySingleQuota = values.autoRecoveryEnabled
      ? quotaFromDisplayAmount(values.autoRecoverySingleAmount)
      : '0'
    const autoRecoveryThresholdQuota = values.autoRecoveryEnabled
      ? quotaFromDisplayAmount(values.autoRecoveryThresholdAmount)
      : '0'
    const autoRecoveryMaxQuota = values.autoRecoveryEnabled
      ? quotaFromDisplayAmount(values.autoRecoveryMaxAmount)
      : '0'
    if (
      !budgetQuota ||
      !initialGrantQuota ||
      recoveryReserveQuota === null ||
      autoRecoverySingleQuota === null ||
      autoRecoveryThresholdQuota === null ||
      autoRecoveryMaxQuota === null
    ) {
      return
    }
    mutation.mutate({
      cycle_start_at: startAt,
      cycle_end_at: endAt,
      budget_quota: budgetQuota,
      initial_grant_quota: initialGrantQuota,
      recovery_reserve_quota: recoveryReserveQuota,
      auto_recovery_enabled: values.autoRecoveryEnabled,
      auto_recovery_single_quota: autoRecoverySingleQuota,
      auto_recovery_threshold_quota: autoRecoveryThresholdQuota,
      auto_recovery_max_count: values.autoRecoveryEnabled
        ? Number(values.autoRecoveryMaxCount)
        : 0,
      auto_recovery_max_quota: autoRecoveryMaxQuota,
      balance_policy: values.balancePolicy,
    })
  })

  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      <DialogContent className='max-h-[90vh] overflow-y-auto sm:max-w-2xl'>
        <DialogHeader>
          <DialogTitle>{t('Create quota cycle')}</DialogTitle>
          <DialogDescription>
            {t(
              'Define the purchasing window and quota pool before generating plans.'
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
                  type='datetime-local'
                  {...form.register('startAt', {
                    onChange: (event) => {
                      const startAt = fromShanghaiInput(event.target.value)
                      if (Number.isFinite(startAt)) {
                        form.setValue(
                          'endAt',
                          toLocalInput(startAt + 30 * 86_400)
                        )
                      }
                    },
                  })}
                />
                <FieldError errors={[form.formState.errors.startAt]} />
              </Field>
              <Field data-invalid={!!form.formState.errors.endAt}>
                <FieldLabel htmlFor='quota-cycle-end'>
                  {t('End time')}
                </FieldLabel>
                <Input
                  id='quota-cycle-end'
                  type='datetime-local'
                  {...form.register('endAt')}
                />
                <FieldError errors={[form.formState.errors.endAt]} />
              </Field>
            </div>
            <Controller
              name='autoRecoveryEnabled'
              control={form.control}
              render={({ field }) => (
                <Field orientation='horizontal'>
                  <div className='flex-1'>
                    <FieldLabel htmlFor='quota-cycle-auto-recovery'>
                      {t('Automatically approve small recovery requests')}
                    </FieldLabel>
                    <FieldDescription>
                      {t(
                        'Disabled by default. Configure every limit before activation; the policy is frozen after the cycle starts.'
                      )}
                    </FieldDescription>
                  </div>
                  <Switch
                    id='quota-cycle-auto-recovery'
                    checked={field.value}
                    onCheckedChange={field.onChange}
                  />
                </Field>
              )}
            />
            {form.watch('autoRecoveryEnabled') && (
              <div className='grid gap-4 sm:grid-cols-2'>
                <Field
                  data-invalid={
                    !!form.formState.errors.autoRecoverySingleAmount
                  }
                >
                  <FieldLabel htmlFor='quota-cycle-auto-single'>
                    {t('Automatic single-request limit')} ({currencyLabel})
                  </FieldLabel>
                  <Input
                    id='quota-cycle-auto-single'
                    inputMode='decimal'
                    {...form.register('autoRecoverySingleAmount')}
                  />
                  <FieldError
                    errors={[form.formState.errors.autoRecoverySingleAmount]}
                  />
                </Field>
                <Field
                  data-invalid={
                    !!form.formState.errors.autoRecoveryThresholdAmount
                  }
                >
                  <FieldLabel htmlFor='quota-cycle-auto-threshold'>
                    {t('Automatic balance threshold')} ({currencyLabel})
                  </FieldLabel>
                  <Input
                    id='quota-cycle-auto-threshold'
                    inputMode='decimal'
                    {...form.register('autoRecoveryThresholdAmount')}
                  />
                  <FieldError
                    errors={[form.formState.errors.autoRecoveryThresholdAmount]}
                  />
                </Field>
                <Field
                  data-invalid={!!form.formState.errors.autoRecoveryMaxCount}
                >
                  <FieldLabel htmlFor='quota-cycle-auto-count'>
                    {t('Automatic requests per user')}
                  </FieldLabel>
                  <Input
                    id='quota-cycle-auto-count'
                    type='number'
                    min={1}
                    step={1}
                    {...form.register('autoRecoveryMaxCount')}
                  />
                  <FieldError
                    errors={[form.formState.errors.autoRecoveryMaxCount]}
                  />
                </Field>
                <Field
                  data-invalid={!!form.formState.errors.autoRecoveryMaxAmount}
                >
                  <FieldLabel htmlFor='quota-cycle-auto-total'>
                    {t('Automatic total per user')} ({currencyLabel})
                  </FieldLabel>
                  <Input
                    id='quota-cycle-auto-total'
                    inputMode='decimal'
                    {...form.register('autoRecoveryMaxAmount')}
                  />
                  <FieldError
                    errors={[form.formState.errors.autoRecoveryMaxAmount]}
                  />
                </Field>
              </div>
            )}
            <Field>
              <FieldLabel>{t('Balance at cycle end')}</FieldLabel>
              <Controller
                name='balancePolicy'
                control={form.control}
                render={({ field }) => {
                  const items = [
                    { label: t('Reset to zero'), value: 'reset' },
                    { label: t('Carry forward'), value: 'carry' },
                  ]
                  return (
                    <Select
                      items={items}
                      value={field.value}
                      onValueChange={field.onChange}
                    >
                      <SelectTrigger className='w-full'>
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectGroup>
                          {items.map((item) => (
                            <SelectItem key={item.value} value={item.value}>
                              {item.label}
                            </SelectItem>
                          ))}
                        </SelectGroup>
                      </SelectContent>
                    </Select>
                  )
                }}
              />
              <FieldDescription>
                {t(
                  'Reset clears managed balances at cycle close; carry keeps them and counts them against the next cycle budget.'
                )}
              </FieldDescription>
            </Field>
            <div className='grid gap-4 sm:grid-cols-2'>
              <Field data-invalid={!!form.formState.errors.budgetAmount}>
                <FieldLabel htmlFor='quota-cycle-budget'>
                  {t('Budget')} ({currencyLabel})
                </FieldLabel>
                <Input
                  id='quota-cycle-budget'
                  inputMode='decimal'
                  placeholder='10000.00'
                  {...form.register('budgetAmount')}
                />
                <FieldError errors={[form.formState.errors.budgetAmount]} />
              </Field>
              <Field data-invalid={!!form.formState.errors.initialGrantAmount}>
                <FieldLabel htmlFor='quota-cycle-grant'>
                  {t('Initial grant per user')} ({currencyLabel})
                </FieldLabel>
                <Input
                  id='quota-cycle-grant'
                  inputMode='decimal'
                  placeholder='100.00'
                  {...form.register('initialGrantAmount')}
                />
                <FieldError
                  errors={[form.formState.errors.initialGrantAmount]}
                />
              </Field>
              <Field
                data-invalid={!!form.formState.errors.recoveryReserveAmount}
              >
                <FieldLabel htmlFor='quota-cycle-recovery-reserve'>
                  {t('Recovery reserve')} ({currencyLabel})
                </FieldLabel>
                <Input
                  id='quota-cycle-recovery-reserve'
                  inputMode='decimal'
                  placeholder='0'
                  {...form.register('recoveryReserveAmount')}
                />
                <FieldError
                  errors={[form.formState.errors.recoveryReserveAmount]}
                />
                <FieldDescription>
                  {t(
                    'Reserved from every ordinary stage for temporary recovery; existing cycles default to zero.'
                  )}
                </FieldDescription>
              </Field>
            </div>
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
