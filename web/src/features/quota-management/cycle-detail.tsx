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
              message: t('请输入正整数'),
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
        queryClient.invalidateQueries({
          queryKey: queryKeys.cycle(props.cycleId),
        }),
        queryClient.invalidateQueries({ queryKey: queryKeys.cycles }),
        queryClient.invalidateQueries({ queryKey: queryKeys.options }),
      ])
      toast.success(t('额度周期已更新'))
      setEditOpen(false)
    },
    onError: (error: unknown) => handleServerError(error),
  })
  const closeMutation = useMutation({
    mutationFn: () => closeCycle(props.cycleId),
    onSuccess: async ({ status }) => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: queryKeys.overview }),
        queryClient.invalidateQueries({ queryKey: queryKeys.cycles }),
        queryClient.invalidateQueries({ queryKey: queryKeys.options }),
      ])
      toast.success(
        status === 'settling'
          ? t('额度周期已开始结算')
          : t('额度周期已关闭')
      )
      await navigate({ to: '/quota-management' })
    },
    onError: (error: unknown) => handleServerError(error),
  })
  if (query.isPending) {
    return (
      <SectionPageLayout>
        <SectionPageLayout.Title>{t('额度周期')}</SectionPageLayout.Title>
        <SectionPageLayout.Content>
          <PageLoading />
        </SectionPageLayout.Content>
      </SectionPageLayout>
    )
  }
  if (query.isError) {
    return (
      <SectionPageLayout>
        <SectionPageLayout.Title>{t('额度周期')}</SectionPageLayout.Title>
        <SectionPageLayout.Content>
          <PageError
            title={t('加载额度周期失败')}
            error={query.error}
          />
        </SectionPageLayout.Content>
      </SectionPageLayout>
    )
  }
  const { cycle, plans, recommendation } = query.data
  const closeCycleDescription =
    cycle.status === 'scheduled'
      ? t('此操作将取消排定的周期及所有草稿，且不会改变用户余额')
      : t('进入周期结算并阻止新请求，待进行中的请求完成后按结转规则处理余额并关闭周期')
  let rolloverDescription = t('周期关闭时清零受管用户余额')
  if (cycle.balance_policy === 'carry') {
    rolloverDescription = cycle.carry_over_counts_toward_budget
      ? t('占用新周期额度')
      : t('不占用新周期额度')
  }

  return (
    <>
      <SectionPageLayout>
        <SectionPageLayout.Title>
          {t('额度周期')} #{cycle.id}
        </SectionPageLayout.Title>
        <SectionPageLayout.Breadcrumb>
          <Button
            variant='ghost'
            size='sm'
            className='text-muted-foreground h-auto px-0 py-1 text-xs'
            render={<Link to='/quota-management' />}
          >
            <HugeiconsIcon icon={ArrowLeft01Icon} strokeWidth={2} />
            {t('返回周期额度管理')}
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
            {t('创建方案')}
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
            {t('编辑周期')}
          </Button>
          <Button
            variant='destructive'
            onClick={() => setCloseOpen(true)}
            disabled={cycle.status === 'closed'}
          >
            {t('关闭周期')}
          </Button>
        </SectionPageLayout.Actions>
        <SectionPageLayout.Content>
          <div className='grid gap-4'>
            {optionsQuery.isError && (
              <PageError
                title={t('生成调配草稿')}
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
                <p className='font-medium'>{t('额度数据异常')}</p>
                <p className='mt-1 text-sm'>
                  {t('已保存的额度数据无效。请联系系统维护人员处理，处理前无法创建或执行方案。用户仍可使用各自账户余额调用 API')}
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
                    {t('分配数据需要初始化')}
                  </p>
                  <p className='mt-1 text-sm'>
                    {t('当前周期需要由系统维护人员完成一次分配数据初始化，之后才能创建或执行方案。用户仍可按账户余额正常使用 API')}
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
                  <CardDescription>{t('周期总额度')}</CardDescription>
                  <CardTitle>{formatQuota(cycle.budget_quota)}</CardTitle>
                </CardHeader>
                <CardContent>{t('本周期可用总额')}</CardContent>
              </Card>
              <Card>
                <CardHeader>
                  <CardDescription>
                    {t('期初已发放额度')}
                  </CardDescription>
                  <CardTitle>
                    {cycle.opening_allocated_quota === null
                      ? '—'
                      : formatQuota(cycle.opening_allocated_quota)}
                  </CardTitle>
                </CardHeader>
                <CardContent>
                  {t('周期开始时登记的受管账户余额')}
                </CardContent>
              </Card>
              <Card>
                <CardHeader>
                  <CardDescription>{t('已分配额度')}</CardDescription>
                  <CardTitle>
                    {cycle.allocated_quota === null
                      ? '—'
                      : formatQuota(cycle.allocated_quota)}
                  </CardTitle>
                </CardHeader>
                <CardContent>
                  {t('周期内已发放且尚未收回')}
                </CardContent>
              </Card>
              <Card>
                <CardHeader>
                  <CardDescription>{t('周期剩余额度')}</CardDescription>
                  <CardTitle>
                    {cycle.remaining_quota === null
                      ? '—'
                      : formatQuota(cycle.remaining_quota)}
                  </CardTitle>
                </CardHeader>
                <CardContent>{t('可用于后续方案')}</CardContent>
              </Card>
              <Card>
                <CardHeader>
                  <CardDescription>
                    {t('每位用户的初始额度')}
                  </CardDescription>
                  <CardTitle>
                    {formatQuota(cycle.initial_grant_quota)}
                  </CardTitle>
                </CardHeader>
                <CardContent>
                  {t('由第一个分配方案使用')}
                </CardContent>
              </Card>
              <Card>
                <CardHeader>
                  <CardDescription>
                    {t('自动分配上限')}
                  </CardDescription>
                  <CardTitle>
                    {cycle.concentration_multiplier_basis_points / 10_000}×
                  </CardTitle>
                </CardHeader>
                <CardContent>
                  {t('本周期每次常规调配都使用这个上限')}
                </CardContent>
              </Card>
              <Card>
                <CardHeader>
                  <CardDescription>
                    {t('临时额度预留')}
                  </CardDescription>
                  <CardTitle>
                    {formatQuota(cycle.temporary_quota_reserve)}
                  </CardTitle>
                </CardHeader>
                <CardContent className='space-y-1'>
                  <div>{t('为临时额度申请预留')}</div>
                  <div>
                    {cycle.temporary_quota_auto_approval_enabled
                      ? t('自动批准已开启')
                      : t('自动批准已关闭')}
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
                  <CardDescription>{t('余额结转')}</CardDescription>
                  <CardTitle>
                    {cycle.balance_policy === 'carry'
                      ? t('结转')
                      : t('到期不结转')}
                  </CardTitle>
                </CardHeader>
                <CardContent>{rolloverDescription}</CardContent>
              </Card>
            </div>
            <Card>
              <CardHeader>
                <CardTitle>{t('额度调配方案')}</CardTitle>
                <CardDescription>
                  {t('每个方案代表本周期内一次可审核的额度调配')}
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
            <DialogTitle>{t('编辑额度周期')}</DialogTitle>
            <DialogDescription>
              {t('周期开始前可修改各项设置，开始后仅可调整周期总额度')}
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
                  {t('预算')} ({currencyLabel})
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
                  {t('每人首次发放额度')} ({currencyLabel})
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
                  {t('临时额度预留')} ({currencyLabel})
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
                        {t('自动批准符合条件的临时额度申请')}
                      </FieldLabel>
                      <FieldDescription>
                        {t('周期开始后策略将被冻结')}
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
                      {t('单次最多自动批准额度')} ({currencyLabel})
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
                      {t('仅在余额低于此值时自动批准')} ({currencyLabel})
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
                      {t('每人最多自动批准次数')}
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
                      {t('每人最多自动批准总额')} ({currencyLabel})
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
            <AlertDialogTitle>{t('关闭此额度周期')}</AlertDialogTitle>
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
              onClick={() => closeMutation.mutate(undefined)}
            >
              {closeMutation.isPending && <Spinner data-icon='inline-start' />}
              {t('关闭周期')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  )
}
