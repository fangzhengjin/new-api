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
import {
  ArrowLeft01Icon,
  Coins01Icon,
  Edit02Icon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, useNavigate } from '@tanstack/react-router'
import { useEffect, useMemo, useState } from 'react'
import { Controller, useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { z } from 'zod'

import { SectionPageLayout } from '@/components/layout'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
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
import { Spinner } from '@/components/ui/spinner'
import { Switch } from '@/components/ui/switch'
import { getCurrencyDisplay, getCurrencyLabel } from '@/lib/currency'
import { handleServerError } from '@/lib/handle-server-error'

import { closeCycle, getCycle, getPlanOptions, updateCycle } from './api'
import { GeneratePlanDialog } from './components/generate-plan-dialog'
import { PlanTable } from './components/plan-table'
import {
  InitialGrantRecommendationAlert,
  PageError,
  PageLoading,
  StatusBadge,
} from './components/shared'
import {
  formatDateTime,
  formatQuota,
  queryKeys,
  quotaFromDisplayAmount,
  quotaToDisplayAmount,
} from './utils'

type FormValues = {
  budgetAmount: string
  initialGrantAmount: string
  temporaryQuotaReserveAmount: string
  temporaryQuotaAutoApprovalEnabled: boolean
  temporaryQuotaAutoApprovalSingleAmount: string
  temporaryQuotaAutoApprovalThresholdAmount: string
  temporaryQuotaAutoApprovalMaxCount: string
  temporaryQuotaAutoApprovalMaxAmount: string
}
const currencyAmountPattern = /^\d+(?:\.\d{1,6})?$/
const tokenAmountPattern = /^\d+$/

export function QuotaCycleDetail(props: { cycleId: number }) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const { meta: currencyMeta } = getCurrencyDisplay()
  const currencyLabel = getCurrencyLabel()
  const tokensOnly = currencyMeta.kind === 'tokens'
  const amountPattern = tokensOnly ? tokenAmountPattern : currencyAmountPattern
  const amountError = tokensOnly
    ? t('Enter quota in tokens')
    : t('Enter quota in {{currency}}', { currency: currencyLabel })
  const [editOpen, setEditOpen] = useState(false)
  const [closeOpen, setCloseOpen] = useState(false)
  const [planOpen, setPlanOpen] = useState(false)
  const query = useQuery({
    queryKey: queryKeys.cycle(props.cycleId),
    queryFn: () => getCycle(props.cycleId),
  })
  const optionsQuery = useQuery({
    queryKey: queryKeys.options,
    queryFn: getPlanOptions,
  })
  const schema = useMemo(
    () =>
      z
        .object({
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
      budgetAmount: '',
      initialGrantAmount: '',
      temporaryQuotaReserveAmount: '0',
      temporaryQuotaAutoApprovalEnabled: false,
      temporaryQuotaAutoApprovalSingleAmount: '0',
      temporaryQuotaAutoApprovalThresholdAmount: '0',
      temporaryQuotaAutoApprovalMaxCount: '0',
      temporaryQuotaAutoApprovalMaxAmount: '0',
    },
  })

  useEffect(() => {
    if (!editOpen || !query.data) return
    form.reset({
      budgetAmount: quotaToDisplayAmount(query.data.cycle.budget_quota),
      initialGrantAmount: quotaToDisplayAmount(
        query.data.cycle.initial_grant_quota
      ),
      temporaryQuotaReserveAmount: quotaToDisplayAmount(
        query.data.cycle.temporary_quota_reserve
      ),
      temporaryQuotaAutoApprovalEnabled:
        query.data.cycle.temporary_quota_auto_approval_enabled,
      temporaryQuotaAutoApprovalSingleAmount: quotaToDisplayAmount(
        query.data.cycle.temporary_quota_auto_approval_single_quota
      ),
      temporaryQuotaAutoApprovalThresholdAmount: quotaToDisplayAmount(
        query.data.cycle.temporary_quota_auto_approval_threshold_quota
      ),
      temporaryQuotaAutoApprovalMaxCount: String(
        query.data.cycle.temporary_quota_auto_approval_max_count
      ),
      temporaryQuotaAutoApprovalMaxAmount: quotaToDisplayAmount(
        query.data.cycle.temporary_quota_auto_approval_max_quota
      ),
    })
  }, [editOpen, form, query.data])

  const editMutation = useMutation({
    mutationFn: (values: FormValues) => {
      const budgetQuota = quotaFromDisplayAmount(values.budgetAmount)
      const initialGrantQuota = quotaFromDisplayAmount(
        values.initialGrantAmount
      )
      const temporaryQuotaReserve = quotaFromDisplayAmount(
        values.temporaryQuotaReserveAmount,
        true
      )
      const temporaryQuotaAutoApprovalSingleQuota =
        values.temporaryQuotaAutoApprovalEnabled
          ? quotaFromDisplayAmount(
              values.temporaryQuotaAutoApprovalSingleAmount
            )
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
      const temporaryQuotaAutoApprovalMaxCount =
        values.temporaryQuotaAutoApprovalEnabled
          ? Number(values.temporaryQuotaAutoApprovalMaxCount)
          : 0
      if (
        !budgetQuota ||
        !initialGrantQuota ||
        temporaryQuotaReserve === null ||
        temporaryQuotaAutoApprovalSingleQuota === null ||
        temporaryQuotaAutoApprovalThresholdQuota === null ||
        temporaryQuotaAutoApprovalMaxQuota === null
      ) {
        throw new Error(amountError)
      }
      return updateCycle(props.cycleId, {
        budget_quota: budgetQuota,
        initial_grant_quota:
          query.data?.cycle.status === 'scheduled'
            ? initialGrantQuota
            : undefined,
        temporary_quota_reserve:
          query.data?.cycle.status === 'scheduled'
            ? temporaryQuotaReserve
            : undefined,
        temporary_quota_auto_approval_enabled:
          query.data?.cycle.status === 'scheduled'
            ? values.temporaryQuotaAutoApprovalEnabled
            : undefined,
        temporary_quota_auto_approval_single_quota:
          query.data?.cycle.status === 'scheduled'
            ? temporaryQuotaAutoApprovalSingleQuota
            : undefined,
        temporary_quota_auto_approval_threshold_quota:
          query.data?.cycle.status === 'scheduled'
            ? temporaryQuotaAutoApprovalThresholdQuota
            : undefined,
        temporary_quota_auto_approval_max_count:
          query.data?.cycle.status === 'scheduled'
            ? temporaryQuotaAutoApprovalMaxCount
            : undefined,
        temporary_quota_auto_approval_max_quota:
          query.data?.cycle.status === 'scheduled'
            ? temporaryQuotaAutoApprovalMaxQuota
            : undefined,
      })
    },
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: queryKeys.overview }),
        queryClient.invalidateQueries({ queryKey: queryKeys.activities }),
        queryClient.invalidateQueries({
          queryKey: queryKeys.cycle(props.cycleId),
        }),
        queryClient.invalidateQueries({ queryKey: queryKeys.cycles }),
        queryClient.invalidateQueries({ queryKey: queryKeys.options }),
      ])
      toast.success(t('Quota cycle updated'))
      setEditOpen(false)
    },
    onError: handleServerError,
  })
  const closeMutation = useMutation({
    mutationFn: () => closeCycle(props.cycleId),
    onSuccess: async ({ status }) => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: queryKeys.overview }),
        queryClient.invalidateQueries({ queryKey: queryKeys.activities }),
        queryClient.invalidateQueries({ queryKey: queryKeys.cycles }),
        queryClient.invalidateQueries({ queryKey: queryKeys.options }),
      ])
      toast.success(
        status === 'settling'
          ? t('Quota cycle settlement started')
          : t('Quota cycle closed')
      )
      await navigate({ to: '/quota-management' })
    },
    onError: handleServerError,
  })
  if (query.isPending) {
    return (
      <SectionPageLayout>
        <SectionPageLayout.Title>{t('Quota cycle')}</SectionPageLayout.Title>
        <SectionPageLayout.Content>
          <PageLoading />
        </SectionPageLayout.Content>
      </SectionPageLayout>
    )
  }
  if (query.isError) {
    return (
      <SectionPageLayout>
        <SectionPageLayout.Title>{t('Quota cycle')}</SectionPageLayout.Title>
        <SectionPageLayout.Content>
          <PageError
            title={t('Failed to load quota cycle')}
            error={query.error}
          />
        </SectionPageLayout.Content>
      </SectionPageLayout>
    )
  }
  const { cycle, plans, recommendation } = query.data
  const closeCycleDescription =
    cycle.status === 'scheduled'
      ? t(
          'This cancels the scheduled cycle and every draft without changing user balances'
        )
      : t(
          'This starts cycle settlement, blocks new requests, handles balances by the rollover rule, and closes the cycle after in-progress requests finish'
        )
  let rolloverDescription = t('Managed balances reset when the cycle closes')
  if (cycle.balance_policy === 'carry') {
    rolloverDescription = cycle.carry_over_counts_toward_budget
      ? t('Counts toward new cycle quota')
      : t('Does not count toward new cycle quota')
  }

  return (
    <>
      <SectionPageLayout>
        <SectionPageLayout.Title>
          {t('Quota cycle')} #{cycle.id}
        </SectionPageLayout.Title>
        <SectionPageLayout.Breadcrumb>
          <Button
            variant='ghost'
            size='sm'
            className='text-muted-foreground h-auto px-0 py-1 text-xs'
            render={<Link to='/quota-management' />}
          >
            <HugeiconsIcon icon={ArrowLeft01Icon} strokeWidth={2} />
            {t('Back to cycle quota management')}
          </Button>
        </SectionPageLayout.Breadcrumb>
        <SectionPageLayout.Actions>
          <Button
            variant='outline'
            onClick={() => setPlanOpen(true)}
            disabled={
              cycle.status === 'closed' ||
              optionsQuery.data?.cycle?.id !== cycle.id
            }
          >
            <HugeiconsIcon
              icon={Coins01Icon}
              strokeWidth={2}
              data-icon='inline-start'
            />
            {t('Create plan')}
          </Button>
          <Button
            variant='outline'
            onClick={() => setEditOpen(true)}
            disabled={cycle.status === 'closed'}
          >
            <HugeiconsIcon
              icon={Edit02Icon}
              strokeWidth={2}
              data-icon='inline-start'
            />
            {t('Edit cycle')}
          </Button>
          <Button
            variant='destructive'
            onClick={() => setCloseOpen(true)}
            disabled={cycle.status === 'closed'}
          >
            {t('Close cycle')}
          </Button>
        </SectionPageLayout.Actions>
        <SectionPageLayout.Content>
          <div className='grid gap-4'>
            {optionsQuery.isError && (
              <PageError
                title={t('Generate plan')}
                error={optionsQuery.error}
              />
            )}
            {recommendation && (
              <InitialGrantRecommendationAlert
                recommendation={recommendation}
                t={t}
              />
            )}
            {cycle.allocation_error && (
              <div
                role='alert'
                className='border-destructive/40 bg-destructive/5 text-destructive rounded-lg border px-4 py-3'
              >
                <p className='font-medium'>{t('Allocation data error')}</p>
                <p className='mt-1 text-sm'>
                  {t(
                    'The saved allocation data is invalid. Contact a system maintainer before creating or executing plans. User API usage remains available from each user balance'
                  )}
                </p>
              </div>
            )}
            {cycle.status === 'active' &&
              !cycle.allocation_error &&
              cycle.allocated_quota === null && (
                <div
                  role='alert'
                  className='border-destructive/40 bg-destructive/5 text-destructive rounded-lg border px-4 py-3'
                >
                  <p className='font-medium'>
                    {t('Allocation data setup required')}
                  </p>
                  <p className='mt-1 text-sm'>
                    {t(
                      'This cycle requires a one-time allocation data setup by a system maintainer before plans can be created or executed. API usage remains available from each user balance'
                    )}
                  </p>
                </div>
              )}
            <div className='grid gap-4 sm:grid-cols-2 lg:grid-cols-4'>
              <Card>
                <CardHeader>
                  <CardDescription>{t('Status')}</CardDescription>
                  <CardTitle>
                    <StatusBadge value={cycle.status} t={t} />
                  </CardTitle>
                </CardHeader>
                <CardContent>
                  {formatDateTime(cycle.cycle_start_at)} –{' '}
                  {formatDateTime(cycle.cycle_end_at)}
                </CardContent>
              </Card>
              <Card>
                <CardHeader>
                  <CardDescription>{t('Cycle total')}</CardDescription>
                  <CardTitle>{formatQuota(cycle.budget_quota)}</CardTitle>
                </CardHeader>
                <CardContent>{t('Total available in this cycle')}</CardContent>
              </Card>
              <Card>
                <CardHeader>
                  <CardDescription>
                    {t('Opening allocated quota')}
                  </CardDescription>
                  <CardTitle>
                    {cycle.opening_allocated_quota === null
                      ? '—'
                      : formatQuota(cycle.opening_allocated_quota)}
                  </CardTitle>
                </CardHeader>
                <CardContent>
                  {t('Managed balances recorded when the cycle started')}
                </CardContent>
              </Card>
              <Card>
                <CardHeader>
                  <CardDescription>{t('Allocated quota')}</CardDescription>
                  <CardTitle>
                    {cycle.allocated_quota === null
                      ? '—'
                      : formatQuota(cycle.allocated_quota)}
                  </CardTitle>
                </CardHeader>
                <CardContent>
                  {t('Issued and not reclaimed in this cycle')}
                </CardContent>
              </Card>
              <Card>
                <CardHeader>
                  <CardDescription>{t('Cycle remaining')}</CardDescription>
                  <CardTitle>
                    {cycle.remaining_quota === null
                      ? '—'
                      : formatQuota(cycle.remaining_quota)}
                  </CardTitle>
                </CardHeader>
                <CardContent>{t('Available for future plans')}</CardContent>
              </Card>
              <Card>
                <CardHeader>
                  <CardDescription>
                    {t('Starting amount per user')}
                  </CardDescription>
                  <CardTitle>
                    {formatQuota(cycle.initial_grant_quota)}
                  </CardTitle>
                </CardHeader>
                <CardContent>
                  {t('Used by the first allocation plan')}
                </CardContent>
              </Card>
              <Card>
                <CardHeader>
                  <CardDescription>
                    {t('Automatic allocation limit')}
                  </CardDescription>
                  <CardTitle>
                    {cycle.concentration_multiplier_basis_points / 10_000}×
                  </CardTitle>
                </CardHeader>
                <CardContent>
                  {t('Used by every regular adjustment in this cycle')}
                </CardContent>
              </Card>
              <Card>
                <CardHeader>
                  <CardDescription>
                    {t('Temporary quota reserve')}
                  </CardDescription>
                  <CardTitle>
                    {formatQuota(cycle.temporary_quota_reserve)}
                  </CardTitle>
                </CardHeader>
                <CardContent className='space-y-1'>
                  <div>{t('Reserved for temporary quota requests')}</div>
                  <div>
                    {cycle.temporary_quota_auto_approval_enabled
                      ? t('Automatic approval enabled')
                      : t('Automatic approval disabled')}
                  </div>
                </CardContent>
              </Card>
              <Card>
                <CardHeader>
                  <CardDescription>{t('Last updated')}</CardDescription>
                  <CardTitle>{formatDateTime(cycle.updated_at)}</CardTitle>
                </CardHeader>
                <CardContent>
                  {cycle.updated_by || cycle.created_by || '—'}
                </CardContent>
              </Card>
              <Card>
                <CardHeader>
                  <CardDescription>{t('Balance rollover')}</CardDescription>
                  <CardTitle>
                    {cycle.balance_policy === 'carry'
                      ? t('Carry over')
                      : t('Reset at close')}
                  </CardTitle>
                </CardHeader>
                <CardContent>{rolloverDescription}</CardContent>
              </Card>
            </div>
            <Card>
              <CardHeader>
                <CardTitle>{t('Allocation plans')}</CardTitle>
                <CardDescription>
                  {t(
                    'Each plan is one reviewable quota allocation in this cycle'
                  )}
                </CardDescription>
              </CardHeader>
              <CardContent>
                <PlanTable plans={plans} />
              </CardContent>
            </Card>
          </div>
        </SectionPageLayout.Content>
      </SectionPageLayout>

      <GeneratePlanDialog
        open={planOpen}
        onOpenChange={setPlanOpen}
        options={optionsQuery.data}
      />

      <Dialog open={editOpen} onOpenChange={setEditOpen}>
        <DialogContent className='max-h-[90vh] overflow-y-auto sm:max-w-2xl'>
          <DialogHeader>
            <DialogTitle>{t('Edit quota cycle')}</DialogTitle>
            <DialogDescription>
              {t(
                'Before the cycle starts, settings can be edited. After it starts, only the cycle total quota can change'
              )}
            </DialogDescription>
          </DialogHeader>
          <form
            id='edit-quota-cycle'
            onSubmit={form.handleSubmit((values) =>
              editMutation.mutate(values)
            )}
          >
            <FieldGroup>
              <Field data-invalid={!!form.formState.errors.budgetAmount}>
                <FieldLabel htmlFor='edit-cycle-budget'>
                  {t('Budget')} ({currencyLabel})
                </FieldLabel>
                <Input
                  id='edit-cycle-budget'
                  aria-invalid={!!form.formState.errors.budgetAmount}
                  aria-describedby={
                    form.formState.errors.budgetAmount
                      ? 'edit-cycle-budget-error'
                      : undefined
                  }
                  inputMode='decimal'
                  {...form.register('budgetAmount')}
                />
                <FieldError
                  id='edit-cycle-budget-error'
                  errors={[form.formState.errors.budgetAmount]}
                />
              </Field>
              <Field
                data-disabled={cycle.status !== 'scheduled'}
                data-invalid={!!form.formState.errors.initialGrantAmount}
              >
                <FieldLabel htmlFor='edit-cycle-grant'>
                  {t('Initial grant per user')} ({currencyLabel})
                </FieldLabel>
                <Input
                  id='edit-cycle-grant'
                  aria-invalid={!!form.formState.errors.initialGrantAmount}
                  aria-describedby={
                    form.formState.errors.initialGrantAmount
                      ? 'edit-cycle-grant-error'
                      : undefined
                  }
                  inputMode='decimal'
                  disabled={cycle.status !== 'scheduled'}
                  {...form.register('initialGrantAmount')}
                />
                <FieldError
                  id='edit-cycle-grant-error'
                  errors={[form.formState.errors.initialGrantAmount]}
                />
              </Field>
              <Field
                data-disabled={cycle.status !== 'scheduled'}
                data-invalid={
                  !!form.formState.errors.temporaryQuotaReserveAmount
                }
              >
                <FieldLabel htmlFor='edit-cycle-temporary-reserve'>
                  {t('Temporary quota reserve')} ({currencyLabel})
                </FieldLabel>
                <Input
                  id='edit-cycle-temporary-reserve'
                  aria-invalid={
                    !!form.formState.errors.temporaryQuotaReserveAmount
                  }
                  aria-describedby={
                    form.formState.errors.temporaryQuotaReserveAmount
                      ? 'edit-cycle-temporary-reserve-error'
                      : undefined
                  }
                  inputMode='decimal'
                  disabled={cycle.status !== 'scheduled'}
                  {...form.register('temporaryQuotaReserveAmount')}
                />
                <FieldError
                  id='edit-cycle-temporary-reserve-error'
                  errors={[form.formState.errors.temporaryQuotaReserveAmount]}
                />
              </Field>
              <Controller
                name='temporaryQuotaAutoApprovalEnabled'
                control={form.control}
                render={({ field }) => (
                  <Field
                    orientation='horizontal'
                    data-disabled={cycle.status !== 'scheduled'}
                  >
                    <div className='flex-1'>
                      <FieldLabel htmlFor='edit-cycle-auto-temporary-quota'>
                        {t(
                          'Automatically approve eligible temporary quota requests'
                        )}
                      </FieldLabel>
                      <FieldDescription>
                        {t('The policy is frozen after the cycle starts')}
                      </FieldDescription>
                    </div>
                    <Switch
                      id='edit-cycle-auto-temporary-quota'
                      checked={field.value}
                      disabled={cycle.status !== 'scheduled'}
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
                    <FieldLabel htmlFor='edit-cycle-auto-single'>
                      {t('Automatic single-request limit')} ({currencyLabel})
                    </FieldLabel>
                    <Input
                      id='edit-cycle-auto-single'
                      aria-invalid={
                        !!form.formState.errors
                          .temporaryQuotaAutoApprovalSingleAmount
                      }
                      aria-describedby={
                        form.formState.errors
                          .temporaryQuotaAutoApprovalSingleAmount
                          ? 'edit-cycle-auto-single-error'
                          : undefined
                      }
                      inputMode='decimal'
                      disabled={cycle.status !== 'scheduled'}
                      {...form.register(
                        'temporaryQuotaAutoApprovalSingleAmount'
                      )}
                    />
                    <FieldError
                      id='edit-cycle-auto-single-error'
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
                    <FieldLabel htmlFor='edit-cycle-auto-threshold'>
                      {t('Automatic balance threshold')} ({currencyLabel})
                    </FieldLabel>
                    <Input
                      id='edit-cycle-auto-threshold'
                      aria-invalid={
                        !!form.formState.errors
                          .temporaryQuotaAutoApprovalThresholdAmount
                      }
                      aria-describedby={
                        form.formState.errors
                          .temporaryQuotaAutoApprovalThresholdAmount
                          ? 'edit-cycle-auto-threshold-error'
                          : undefined
                      }
                      inputMode='decimal'
                      disabled={cycle.status !== 'scheduled'}
                      {...form.register(
                        'temporaryQuotaAutoApprovalThresholdAmount'
                      )}
                    />
                    <FieldError
                      id='edit-cycle-auto-threshold-error'
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
                    <FieldLabel htmlFor='edit-cycle-auto-count'>
                      {t('Automatic requests per user')}
                    </FieldLabel>
                    <Input
                      id='edit-cycle-auto-count'
                      aria-invalid={
                        !!form.formState.errors
                          .temporaryQuotaAutoApprovalMaxCount
                      }
                      aria-describedby={
                        form.formState.errors.temporaryQuotaAutoApprovalMaxCount
                          ? 'edit-cycle-auto-count-error'
                          : undefined
                      }
                      type='number'
                      min={1}
                      step={1}
                      disabled={cycle.status !== 'scheduled'}
                      {...form.register('temporaryQuotaAutoApprovalMaxCount')}
                    />
                    <FieldError
                      id='edit-cycle-auto-count-error'
                      errors={[
                        form.formState.errors
                          .temporaryQuotaAutoApprovalMaxCount,
                      ]}
                    />
                  </Field>
                  <Field
                    data-invalid={
                      !!form.formState.errors
                        .temporaryQuotaAutoApprovalMaxAmount
                    }
                  >
                    <FieldLabel htmlFor='edit-cycle-auto-total'>
                      {t('Automatic total per user')} ({currencyLabel})
                    </FieldLabel>
                    <Input
                      id='edit-cycle-auto-total'
                      aria-invalid={
                        !!form.formState.errors
                          .temporaryQuotaAutoApprovalMaxAmount
                      }
                      aria-describedby={
                        form.formState.errors
                          .temporaryQuotaAutoApprovalMaxAmount
                          ? 'edit-cycle-auto-total-error'
                          : undefined
                      }
                      inputMode='decimal'
                      disabled={cycle.status !== 'scheduled'}
                      {...form.register('temporaryQuotaAutoApprovalMaxAmount')}
                    />
                    <FieldError
                      id='edit-cycle-auto-total-error'
                      errors={[
                        form.formState.errors
                          .temporaryQuotaAutoApprovalMaxAmount,
                      ]}
                    />
                  </Field>
                </div>
              )}
            </FieldGroup>
          </form>
          <DialogFooter>
            <Button
              type='button'
              variant='outline'
              disabled={editMutation.isPending}
              onClick={() => setEditOpen(false)}
            >
              {t('Cancel')}
            </Button>
            <Button
              type='submit'
              form='edit-quota-cycle'
              disabled={editMutation.isPending}
            >
              {editMutation.isPending && <Spinner data-icon='inline-start' />}
              {t('Save')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <AlertDialog open={closeOpen} onOpenChange={setCloseOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t('Close this quota cycle?')}</AlertDialogTitle>
            <AlertDialogDescription>
              {closeCycleDescription}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={closeMutation.isPending}>
              {t('Cancel')}
            </AlertDialogCancel>
            <AlertDialogAction
              variant='destructive'
              disabled={closeMutation.isPending}
              onClick={() => closeMutation.mutate()}
            >
              {closeMutation.isPending && <Spinner data-icon='inline-start' />}
              {t('Close cycle')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  )
}
