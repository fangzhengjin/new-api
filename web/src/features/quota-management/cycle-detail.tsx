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
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { Spinner } from '@/components/ui/spinner'
import { Switch } from '@/components/ui/switch'
import { getCurrencyDisplay, getCurrencyLabel } from '@/lib/currency'
import { handleServerError } from '@/lib/handle-server-error'

import {
  closeCycle,
  getCycle,
  getPlanOptions,
  restoreCycle,
  updateCycle,
} from './api'
import { GeneratePlanDialog } from './components/generate-plan-dialog'
import { PlanTable } from './components/plan-table'
import {
  InitialGrantRecommendationAlert,
  PageError,
  PageLoading,
  StatusBadge,
} from './components/shared'
import type { ConcentrationMultiplier } from './types'
import {
  formatDateTime,
  formatConcentrationMultiplier,
  formatQuota,
  queryKeys,
  quotaFromDisplayAmount,
  quotaToDisplayAmount,
} from './utils'

type FormValues = {
  budgetAmount: string
  initialGrantAmount: string
  recoveryReserveAmount: string
  autoRecoveryEnabled: boolean
  autoRecoverySingleAmount: string
  autoRecoveryThresholdAmount: string
  autoRecoveryMaxCount: string
  autoRecoveryMaxAmount: string
  concentrationMultiplier: string
}
const currencyAmountPattern = /^\d+(?:\.\d{1,6})?$/
const tokenAmountPattern = /^\d+$/
const concentrationMultiplierPattern = /^(0|15000|20000|30000)$/

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
  const [restoreOpen, setRestoreOpen] = useState(false)
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
          concentrationMultiplier: z
            .string()
            .regex(
              concentrationMultiplierPattern,
              t('Select an automatic allocation limit')
            ),
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
      budgetAmount: '',
      initialGrantAmount: '',
      recoveryReserveAmount: '0',
      autoRecoveryEnabled: false,
      autoRecoverySingleAmount: '0',
      autoRecoveryThresholdAmount: '0',
      autoRecoveryMaxCount: '0',
      autoRecoveryMaxAmount: '0',
      concentrationMultiplier: '0',
    },
  })

  useEffect(() => {
    if (!editOpen || !query.data) return
    form.reset({
      budgetAmount: quotaToDisplayAmount(query.data.cycle.budget_quota),
      initialGrantAmount: quotaToDisplayAmount(
        query.data.cycle.initial_grant_quota
      ),
      recoveryReserveAmount: quotaToDisplayAmount(
        query.data.cycle.recovery_reserve_quota
      ),
      autoRecoveryEnabled: query.data.cycle.auto_recovery_enabled,
      autoRecoverySingleAmount: quotaToDisplayAmount(
        query.data.cycle.auto_recovery_single_quota
      ),
      autoRecoveryThresholdAmount: quotaToDisplayAmount(
        query.data.cycle.auto_recovery_threshold_quota
      ),
      autoRecoveryMaxCount: String(query.data.cycle.auto_recovery_max_count),
      autoRecoveryMaxAmount: quotaToDisplayAmount(
        query.data.cycle.auto_recovery_max_quota
      ),
      concentrationMultiplier:
        query.data.cycle.status === 'scheduled' &&
        query.data.cycle.concentration_multiplier_basis_points === 0
          ? ''
          : String(query.data.cycle.concentration_multiplier_basis_points),
    })
  }, [editOpen, form, query.data])

  const editMutation = useMutation({
    mutationFn: (values: FormValues) => {
      const budgetQuota = quotaFromDisplayAmount(values.budgetAmount)
      const initialGrantQuota = quotaFromDisplayAmount(
        values.initialGrantAmount
      )
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
      const autoRecoveryMaxCount = values.autoRecoveryEnabled
        ? Number(values.autoRecoveryMaxCount)
        : 0
      if (
        !budgetQuota ||
        !initialGrantQuota ||
        recoveryReserveQuota === null ||
        autoRecoverySingleQuota === null ||
        autoRecoveryThresholdQuota === null ||
        autoRecoveryMaxQuota === null
      ) {
        throw new Error(amountError)
      }
      return updateCycle(props.cycleId, {
        budget_quota: budgetQuota,
        initial_grant_quota:
          query.data?.cycle.status === 'scheduled'
            ? initialGrantQuota
            : undefined,
        recovery_reserve_quota:
          query.data?.cycle.status === 'scheduled'
            ? recoveryReserveQuota
            : undefined,
        auto_recovery_enabled:
          query.data?.cycle.status === 'scheduled'
            ? values.autoRecoveryEnabled
            : undefined,
        auto_recovery_single_quota:
          query.data?.cycle.status === 'scheduled'
            ? autoRecoverySingleQuota
            : undefined,
        auto_recovery_threshold_quota:
          query.data?.cycle.status === 'scheduled'
            ? autoRecoveryThresholdQuota
            : undefined,
        auto_recovery_max_count:
          query.data?.cycle.status === 'scheduled'
            ? autoRecoveryMaxCount
            : undefined,
        auto_recovery_max_quota:
          query.data?.cycle.status === 'scheduled'
            ? autoRecoveryMaxQuota
            : undefined,
        concentration_multiplier_basis_points:
          query.data?.cycle.status === 'scheduled'
            ? (Number(
                values.concentrationMultiplier
              ) as ConcentrationMultiplier)
            : undefined,
      })
    },
    onSuccess: async () => {
      await Promise.all([
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
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: queryKeys.cycles }),
        queryClient.invalidateQueries({ queryKey: queryKeys.plans }),
        queryClient.invalidateQueries({ queryKey: queryKeys.options }),
      ])
      toast.success(t('Quota cycle closed'))
      await navigate({ to: '/quota-management' })
    },
    onError: handleServerError,
  })
  const restoreMutation = useMutation({
    mutationFn: () => restoreCycle(props.cycleId),
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({
          queryKey: queryKeys.cycle(props.cycleId),
        }),
        queryClient.invalidateQueries({ queryKey: queryKeys.cycles }),
        queryClient.invalidateQueries({ queryKey: queryKeys.plans }),
        queryClient.invalidateQueries({ queryKey: queryKeys.options }),
      ])
      toast.success(t('Cycle reset snapshot restored'))
      setRestoreOpen(false)
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
  let closeCycleDescription = t(
    'This closes the cycle and cancels every remaining draft. Managed balances carry forward unchanged.'
  )
  if (cycle.status === 'scheduled') {
    closeCycleDescription = t(
      'This cancels the scheduled cycle and every draft without changing balances.'
    )
  } else if (cycle.balance_policy === 'reset') {
    closeCycleDescription = t(
      'This closes the cycle, cancels every remaining draft, and resets all managed balances to zero. Whitelist balances are unchanged.'
    )
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
            {t('Back to quota management')}
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
            {t('Generate plan')}
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
          {cycle.status === 'closed' &&
            cycle.balance_policy === 'reset' &&
            cycle.settlement_plan_id !== null &&
            cycle.restored_at === null && (
              <Button
                variant='outline'
                onClick={() => setRestoreOpen(true)}
                disabled={restoreMutation.isPending}
              >
                {t('Restore reset snapshot')}
              </Button>
            )}
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
            <div className='grid gap-4 sm:grid-cols-2 lg:grid-cols-4'>
              <Card>
                <CardHeader>
                  <CardDescription>{t('Balance at cycle end')}</CardDescription>
                  <CardTitle>
                    {cycle.balance_policy === 'reset'
                      ? t('Reset to zero')
                      : t('Carry forward')}
                  </CardTitle>
                </CardHeader>
                <CardContent>
                  {cycle.restored_at
                    ? t('Reset snapshot restored')
                    : t('Policy fixed at creation')}
                </CardContent>
              </Card>
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
                  <CardDescription>{t('Budget')}</CardDescription>
                  <CardTitle>{formatQuota(cycle.budget_quota)}</CardTitle>
                </CardHeader>
                <CardContent>{t('Purchasing pool')}</CardContent>
              </Card>
              <Card>
                <CardHeader>
                  <CardDescription>
                    {t('Initial grant per user')}
                  </CardDescription>
                  <CardTitle>
                    {formatQuota(cycle.initial_grant_quota)}
                  </CardTitle>
                </CardHeader>
                <CardContent>{t('Applied by initialization')}</CardContent>
              </Card>
              <Card>
                <CardHeader>
                  <CardDescription>
                    {t('Automatic allocation limit')}
                  </CardDescription>
                  <CardTitle>
                    {cycle.concentration_multiplier_basis_points
                      ? formatConcentrationMultiplier(
                          cycle.concentration_multiplier_basis_points
                        )
                      : t('Not set')}
                  </CardTitle>
                </CardHeader>
                <CardContent>
                  {cycle.concentration_multiplier_basis_points
                    ? t('Used by every regular adjustment in this cycle')
                    : t('Uses previous allocation rules')}
                </CardContent>
              </Card>
              <Card>
                <CardHeader>
                  <CardDescription>{t('Recovery reserve')}</CardDescription>
                  <CardTitle>
                    {formatQuota(cycle.recovery_reserve_quota)}
                  </CardTitle>
                </CardHeader>
                <CardContent className='space-y-1'>
                  <div>{t('Excluded from ordinary plans')}</div>
                  <div>
                    {cycle.auto_recovery_enabled
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
            </div>
            <Card>
              <CardHeader>
                <CardTitle>{t('Related quota plans')}</CardTitle>
                <CardDescription>
                  {t('Drafts and execution history for this purchasing cycle.')}
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
              {t('Active cycles allow budget changes only.')}
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
                  inputMode='decimal'
                  {...form.register('budgetAmount')}
                />
                <FieldError errors={[form.formState.errors.budgetAmount]} />
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
                  inputMode='decimal'
                  disabled={cycle.status !== 'scheduled'}
                  {...form.register('initialGrantAmount')}
                />
                <FieldError
                  errors={[form.formState.errors.initialGrantAmount]}
                />
              </Field>
              <Field
                data-disabled={cycle.status !== 'scheduled'}
                data-invalid={!!form.formState.errors.concentrationMultiplier}
              >
                <FieldLabel htmlFor='edit-cycle-concentration-multiplier'>
                  {t('Automatic allocation limit')}
                </FieldLabel>
                <NativeSelect
                  id='edit-cycle-concentration-multiplier'
                  className='w-full'
                  disabled={cycle.status !== 'scheduled'}
                  aria-invalid={!!form.formState.errors.concentrationMultiplier}
                  {...form.register('concentrationMultiplier')}
                >
                  {cycle.concentration_multiplier_basis_points === 0 && (
                    <NativeSelectOption value='0' disabled>
                      {t('Not set')}
                    </NativeSelectOption>
                  )}
                  {cycle.status === 'scheduled' &&
                    cycle.concentration_multiplier_basis_points === 0 && (
                      <NativeSelectOption value='' disabled>
                        {t('Select an automatic allocation limit')}
                      </NativeSelectOption>
                    )}
                  <NativeSelectOption value='15000'>1.5×</NativeSelectOption>
                  <NativeSelectOption value='20000'>2×</NativeSelectOption>
                  <NativeSelectOption value='30000'>3×</NativeSelectOption>
                </NativeSelect>
                <FieldDescription>
                  {t(
                    'The setting can be changed before the cycle starts and is fixed after activation.'
                  )}
                </FieldDescription>
                <FieldError
                  errors={[form.formState.errors.concentrationMultiplier]}
                />
              </Field>
              <Field
                data-disabled={cycle.status !== 'scheduled'}
                data-invalid={!!form.formState.errors.recoveryReserveAmount}
              >
                <FieldLabel htmlFor='edit-cycle-recovery-reserve'>
                  {t('Recovery reserve')} ({currencyLabel})
                </FieldLabel>
                <Input
                  id='edit-cycle-recovery-reserve'
                  inputMode='decimal'
                  disabled={cycle.status !== 'scheduled'}
                  {...form.register('recoveryReserveAmount')}
                />
                <FieldError
                  errors={[form.formState.errors.recoveryReserveAmount]}
                />
              </Field>
              <Controller
                name='autoRecoveryEnabled'
                control={form.control}
                render={({ field }) => (
                  <Field
                    orientation='horizontal'
                    data-disabled={cycle.status !== 'scheduled'}
                  >
                    <div className='flex-1'>
                      <FieldLabel htmlFor='edit-cycle-auto-recovery'>
                        {t('Automatically approve small recovery requests')}
                      </FieldLabel>
                      <FieldDescription>
                        {t('The policy is frozen after the cycle starts.')}
                      </FieldDescription>
                    </div>
                    <Switch
                      id='edit-cycle-auto-recovery'
                      checked={field.value}
                      disabled={cycle.status !== 'scheduled'}
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
                    <FieldLabel htmlFor='edit-cycle-auto-single'>
                      {t('Automatic single-request limit')} ({currencyLabel})
                    </FieldLabel>
                    <Input
                      id='edit-cycle-auto-single'
                      inputMode='decimal'
                      disabled={cycle.status !== 'scheduled'}
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
                    <FieldLabel htmlFor='edit-cycle-auto-threshold'>
                      {t('Automatic balance threshold')} ({currencyLabel})
                    </FieldLabel>
                    <Input
                      id='edit-cycle-auto-threshold'
                      inputMode='decimal'
                      disabled={cycle.status !== 'scheduled'}
                      {...form.register('autoRecoveryThresholdAmount')}
                    />
                    <FieldError
                      errors={[
                        form.formState.errors.autoRecoveryThresholdAmount,
                      ]}
                    />
                  </Field>
                  <Field
                    data-invalid={!!form.formState.errors.autoRecoveryMaxCount}
                  >
                    <FieldLabel htmlFor='edit-cycle-auto-count'>
                      {t('Automatic requests per user')}
                    </FieldLabel>
                    <Input
                      id='edit-cycle-auto-count'
                      type='number'
                      min={1}
                      step={1}
                      disabled={cycle.status !== 'scheduled'}
                      {...form.register('autoRecoveryMaxCount')}
                    />
                    <FieldError
                      errors={[form.formState.errors.autoRecoveryMaxCount]}
                    />
                  </Field>
                  <Field
                    data-invalid={!!form.formState.errors.autoRecoveryMaxAmount}
                  >
                    <FieldLabel htmlFor='edit-cycle-auto-total'>
                      {t('Automatic total per user')} ({currencyLabel})
                    </FieldLabel>
                    <Input
                      id='edit-cycle-auto-total'
                      inputMode='decimal'
                      disabled={cycle.status !== 'scheduled'}
                      {...form.register('autoRecoveryMaxAmount')}
                    />
                    <FieldError
                      errors={[form.formState.errors.autoRecoveryMaxAmount]}
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

      <AlertDialog open={restoreOpen} onOpenChange={setRestoreOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              {t('Restore this cycle reset snapshot?')}
            </AlertDialogTitle>
            <AlertDialogDescription>
              {t(
                'Recovery succeeds only if every affected balance is still unchanged and the active cycle budget can cover the full snapshot. It is rejected otherwise.'
              )}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={restoreMutation.isPending}>
              {t('Cancel')}
            </AlertDialogCancel>
            <AlertDialogAction
              disabled={restoreMutation.isPending}
              onClick={() => restoreMutation.mutate()}
            >
              {restoreMutation.isPending && (
                <Spinner data-icon='inline-start' />
              )}
              {t('Restore snapshot')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  )
}
