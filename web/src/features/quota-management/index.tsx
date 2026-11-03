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
import {
  ArrowRight01Icon,
  InformationCircleIcon,
  PlusSignIcon,
  RefreshIcon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { SectionPageLayout } from '@/components/layout'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Progress } from '@/components/ui/progress'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { useSystemConfigStore } from '@/stores/system-config-store'

import { getQuotaOverview, listCycles } from './api'
import { CreateCycleDialog } from './components/create-cycle-dialog'
import { CycleTable } from './components/cycle-table'
import {
  EmptyData,
  PageError,
  PageLoading,
  StatusBadge,
} from './components/shared'
import { TemporaryQuotaRequestsCard } from './components/temporary-quota-requests-card'
import type { QuotaCycle, QuotaOverview } from './types'
import { formatDateTime, formatQuota, queryKeys } from './utils'

function CycleSlot(props: {
  label: string
  cycle?: QuotaCycle
  emptyText: string
  onCreate?: () => void
}) {
  const { t } = useTranslation()
  return (
    <div className='bg-muted/30 min-w-0 rounded-lg border p-3'>
      <div className='text-muted-foreground mb-2 text-xs font-medium'>
        {props.label}
      </div>
      {props.cycle ? (
        <div className='space-y-2'>
          <div className='flex items-center justify-between gap-2'>
            <span className='truncate font-medium'>
              {t('周期')} #{props.cycle.id}
            </span>
            <StatusBadge value={props.cycle.status} t={t} />
          </div>
          <div className='text-muted-foreground text-xs tabular-nums'>
            {formatDateTime(props.cycle.cycle_start_at)} –{' '}
            {formatDateTime(props.cycle.cycle_end_at)}
          </div>
          <Button
            size='sm'
            variant='ghost'
            className='-ml-2'
            render={
              <Link
                to='/quota-management/cycles/$cycleId'
                params={{ cycleId: String(props.cycle.id) }}
              />
            }
          >
            {t('View details')}
            <HugeiconsIcon
              icon={ArrowRight01Icon}
              strokeWidth={2}
              data-icon='inline-end'
            />
          </Button>
        </div>
      ) : (
        <div className='space-y-2'>
          <p className='text-muted-foreground text-sm'>{props.emptyText}</p>
          {props.onCreate && (
            <Button size='sm' variant='outline' onClick={props.onCreate}>
              <HugeiconsIcon
                icon={PlusSignIcon}
                strokeWidth={2}
                data-icon='inline-start'
              />
              {t('创建下一周期')}
            </Button>
          )}
        </div>
      )}
    </div>
  )
}

function MetricCard(props: {
  label: string
  value: string | null | undefined
  description: string
  updatedAt?: number
  onRefresh?: () => void
  refreshing?: boolean
}) {
  const { t } = useTranslation()
  return (
    <Card size='sm'>
      <CardHeader>
        <CardDescription className='flex items-center gap-1'>
          {props.label}
          {props.updatedAt && props.value != null && (
            <Tooltip>
              <TooltipTrigger
                aria-label={t('数据更新时间')}
                render={
                  <button
                    type='button'
                    className='focus-visible:ring-ring rounded-sm focus-visible:ring-2 focus-visible:outline-none'
                  />
                }
              >
                <HugeiconsIcon
                  icon={InformationCircleIcon}
                  className='size-3.5'
                  strokeWidth={2}
                />
              </TooltipTrigger>
              <TooltipContent>
                {t('更新于 {{time}}', {
                  time: formatDateTime(props.updatedAt),
                })}
              </TooltipContent>
            </Tooltip>
          )}
        </CardDescription>
        <CardTitle className='flex items-center gap-1.5 text-2xl font-semibold tabular-nums'>
          {props.value === null || props.value === undefined
            ? '—'
            : formatQuota(props.value)}
          {props.value != null && props.onRefresh && (
            <Button
              size='icon-xs'
              variant='ghost'
              aria-label={t('刷新周期剩余额度')}
              disabled={props.refreshing}
              onClick={props.onRefresh}
            >
              <HugeiconsIcon
                icon={RefreshIcon}
                className={props.refreshing ? 'animate-spin' : undefined}
                strokeWidth={2}
              />
            </Button>
          )}
        </CardTitle>
      </CardHeader>
      <CardContent className='text-muted-foreground text-xs'>
        {props.description}
      </CardContent>
    </Card>
  )
}

function FundFlow(props: { overview: QuotaOverview }) {
  const { t } = useTranslation()
  const cycle = props.overview.current_cycle
  const summary = props.overview.current_plan_summary
  if (!cycle) {
    return (
      <EmptyData
        title={t('没有活跃周期')}
        description={t('创建周期以开始管理额度分配')}
      />
    )
  }
  if (cycle.allocation_error) {
    return (
      <EmptyData
        title={t('额度数据异常')}
        description={t('已保存的额度数据无效。请联系系统维护人员处理，处理前无法创建或执行方案。用户仍可使用各自账户余额调用 API')}
      />
    )
  }
  if (cycle.allocated_quota == null || cycle.remaining_quota == null) {
    return (
      <EmptyData
        title={t('分配数据需要初始化')}
        description={t('当前周期需要由系统维护人员完成一次分配数据初始化，之后才能创建或执行方案。用户仍可按账户余额正常使用 API')}
      />
    )
  }
  const budget = Number(cycle.budget_quota)
  const allocated = Number(cycle.allocated_quota ?? 0)
  const allocationRate =
    Number.isFinite(budget) && budget > 0
      ? Math.min(100, Math.max(0, (allocated / budget) * 100))
      : 0
  const rows = [
    {
      label: t('本方案发放'),
      value: summary?.increase.total ?? '0',
    },
    {
      label: t('本方案回收'),
      value: summary?.decrease.total ?? '0',
    },
    {
      label: t('从回收额度中重新发放'),
      value: summary?.reclaimed_used_for_increases ?? '0',
    },
    {
      label: t('收回用于冲抵原有超额'),
      value: summary?.reclaimed_used_to_cover_overage ?? '0',
    },
    {
      label: t('使用周期剩余额度发放'),
      value: summary?.cycle_pool_issued_quota ?? '0',
    },
    {
      label: t('收回额度返还周期额度池'),
      value: summary?.reclaimed_unused ?? '0',
    },
    {
      label: t('本方案执行后周期剩余额度'),
      value: summary?.pool_remaining ?? cycle.remaining_quota,
    },
  ]
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('额度流向')}</CardTitle>
        <CardDescription>
          {t('查看周期额度的发放、收回、重新发放和剩余情况')}
        </CardDescription>
      </CardHeader>
      <CardContent className='space-y-5'>
        {props.overview.current_plan && (
          <div className='flex flex-col justify-between gap-3 rounded-lg border p-3 sm:flex-row sm:items-center'>
            <div className='min-w-0'>
              <div className='text-muted-foreground text-xs'>
                {t('当前方案')}
              </div>
              <div className='mt-1 truncate font-medium'>
                {props.overview.current_plan.name}
              </div>
              {props.overview.current_plan.purpose && (
                <div className='text-muted-foreground mt-0.5 truncate text-xs'>
                  {props.overview.current_plan.purpose}
                </div>
              )}
            </div>
            <StatusBadge value={props.overview.current_plan.status} t={t} />
          </div>
        )}
        <div>
          <div className='mb-2 flex items-center justify-between gap-4 text-sm'>
            <span>{t('周期总额度')}</span>
            <span className='font-medium tabular-nums'>
              {formatQuota(cycle.budget_quota)}
            </span>
          </div>
          <Progress
            value={allocationRate}
            aria-label={t('分配进度')}
          />
          <div className='text-muted-foreground mt-2 flex justify-between gap-4 text-xs'>
            <span>
              {t('已分配')} {formatQuota(cycle.allocated_quota ?? '0')}
            </span>
            <span>
              {t('Remaining')} {formatQuota(cycle.remaining_quota ?? '0')}
            </span>
          </div>
        </div>
        <div className='grid gap-2 sm:grid-cols-2 xl:grid-cols-3'>
          {rows.map((row) => (
            <div
              key={row.label}
              className='bg-muted/40 flex items-center justify-between gap-4 rounded-lg px-3 py-2.5'
            >
              <span className='text-muted-foreground text-xs'>{row.label}</span>
              <span className='font-medium tabular-nums'>
                {row.value == null ? '—' : formatQuota(row.value)}
              </span>
            </div>
          ))}
        </div>
        {props.overview.current_plan && (
          <Button
            variant='outline'
            render={
              <Link
                to='/quota-management/plans/$planId'
                params={{ planId: String(props.overview.current_plan.id) }}
              />
            }
          >
            {t('查看当前方案额度流向')}
            <HugeiconsIcon
              icon={ArrowRight01Icon}
              strokeWidth={2}
              data-icon='inline-end'
            />
          </Button>
        )}
      </CardContent>
    </Card>
  )
}

export function QuotaManagement() {
  const { t } = useTranslation()
  const cycleQuotaManagementEnabled = useSystemConfigStore(
    (state) => state.config.cycleQuotaManagementEnabled === true
  )
  const [cycleDialogOpen, setCycleDialogOpen] = useState(false)
  const overviewQuery = useQuery({
    queryKey: queryKeys.overview,
    queryFn: getQuotaOverview,
  })
  const cyclesQuery = useQuery({
    queryKey: queryKeys.cycles,
    queryFn: listCycles,
  })
  const pendingTotal = useMemo(() => {
    const overview = overviewQuery.data
    if (!overview) return 0
    return (
      overview.pending_temporary_quota_count +
      overview.pending_allocation_count +
      overview.pending_reclaim_count
    )
  }, [overviewQuery.data])

  let cyclesContent
  if (cyclesQuery.isPending) {
    cyclesContent = <PageLoading />
  } else if (cyclesQuery.isError) {
    cyclesContent = (
      <PageError
        title={t('加载额度周期列表失败')}
        error={cyclesQuery.error}
      />
    )
  } else {
    cyclesContent = <CycleTable cycles={cyclesQuery.data.cycles} />
  }
  return (
    <>
      <SectionPageLayout>
        <SectionPageLayout.Title>
          <span className='inline-flex min-w-0 items-center gap-2'>
            <span className='truncate'>{t('周期额度管理')}</span>
            <Badge variant='outline' className='shrink-0'>
              {t('Admin')}
            </Badge>
          </span>
        </SectionPageLayout.Title>
        <SectionPageLayout.Actions>
          <Button
            variant='outline'
            onClick={() => setCycleDialogOpen(true)}
            disabled={!cyclesQuery.data}
          >
            <HugeiconsIcon
              icon={PlusSignIcon}
              strokeWidth={2}
              data-icon='inline-start'
            />
            {t('创建周期')}
          </Button>
        </SectionPageLayout.Actions>
        <SectionPageLayout.Content>
          {overviewQuery.isPending && <PageLoading />}
          {overviewQuery.isError && (
            <PageError
              title={t('加载额度概览失败')}
              error={overviewQuery.error}
            />
          )}
          {overviewQuery.data && (
            <div className='space-y-6'>
              <section aria-labelledby='cycle-navigation-title'>
                <div className='mb-3 flex items-center justify-between gap-3'>
                  <div>
                    <h2 id='cycle-navigation-title' className='font-medium'>
                      {t('周期概览')}
                    </h2>
                    <p className='text-muted-foreground text-sm'>
                      {t('周期定义有效时间范围，方案执行实际额度调配')}
                    </p>
                  </div>
                </div>
                <div className='grid gap-3 md:grid-cols-3'>
                  <CycleSlot
                    label={t('上一周期')}
                    cycle={overviewQuery.data.previous_cycle}
                    emptyText={t('暂无上一周期')}
                  />
                  <CycleSlot
                    label={t('当前周期')}
                    cycle={overviewQuery.data.current_cycle}
                    emptyText={t('没有活跃周期')}
                  />
                  <CycleSlot
                    label={t('下一周期')}
                    cycle={overviewQuery.data.next_cycle}
                    emptyText={t('尚未创建')}
                    onCreate={
                      overviewQuery.data.next_cycle || !cyclesQuery.data
                        ? undefined
                        : () => setCycleDialogOpen(true)
                    }
                  />
                </div>
              </section>

              {overviewQuery.data.current_cycle?.allocation_error && (
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

              {overviewQuery.data.current_cycle &&
                !overviewQuery.data.current_cycle.allocation_error &&
                !overviewQuery.data.allocation_ready && (
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

              <section aria-labelledby='allocation-summary-title'>
                <div className='mb-3 flex items-center justify-between gap-3'>
                  <h2 id='allocation-summary-title' className='font-medium'>
                    {t('分配概况')}
                  </h2>
                  {pendingTotal > 0 && (
                    <Badge variant='outline'>
                      {t('{{count}} 项待处理', { count: pendingTotal })}
                    </Badge>
                  )}
                </div>
                <div className='grid gap-3 md:grid-cols-3'>
                  <MetricCard
                    label={t('周期总额度')}
                    value={overviewQuery.data.current_cycle?.budget_quota}
                    description={t('当前周期内可用的总额度')}
                  />
                  <MetricCard
                    label={t('已分配额度')}
                    value={overviewQuery.data.current_cycle?.allocated_quota}
                    description={t('周期内已发放且尚未收回的额度')}
                  />
                  <MetricCard
                    label={t('周期剩余额度')}
                    value={overviewQuery.data.current_cycle?.remaining_quota}
                    description={t('可用于后续方案和临时额度申请的剩余额度')}
                    updatedAt={overviewQuery.data.updated_at}
                    onRefresh={() => void overviewQuery.refetch()}
                    refreshing={overviewQuery.isFetching}
                  />
                </div>
              </section>

              <div className='grid gap-4 xl:grid-cols-[minmax(0,1.5fr)_minmax(22rem,1fr)]'>
                <FundFlow overview={overviewQuery.data} />
                <Card>
                  <CardHeader>
                    <CardTitle>{t('需要关注的事项')}</CardTitle>
                    <CardDescription>
                      {t('仅显示可能影响下一次调配的事项')}
                    </CardDescription>
                    {overviewQuery.data.current_plan && (
                      <CardAction>
                        <StatusBadge
                          value={overviewQuery.data.current_plan.status}
                          t={t}
                        />
                      </CardAction>
                    )}
                  </CardHeader>
                  <CardContent className='space-y-3'>
                    {[
                      [
                        t('待发放用户'),
                        overviewQuery.data.pending_allocation_count,
                      ],
                      [
                        t('待回收余额'),
                        overviewQuery.data.pending_reclaim_count,
                      ],
                      [
                        t('临时额度申请'),
                        overviewQuery.data.pending_temporary_quota_count,
                      ],
                    ].map(([label, value]) => (
                      <div
                        key={String(label)}
                        className='flex items-center justify-between gap-4 rounded-lg border px-3 py-2.5'
                      >
                        <span>{label}</span>
                        <Badge
                          variant={Number(value) > 0 ? 'outline' : 'secondary'}
                        >
                          {value}
                        </Badge>
                      </div>
                    ))}
                  </CardContent>
                </Card>
              </div>

              <section aria-labelledby='cycle-list-title'>
                <h2 id='cycle-list-title' className='mb-3 font-medium'>
                  {t('所有周期')}
                </h2>
                {cyclesContent}
              </section>

              {cycleQuotaManagementEnabled && <TemporaryQuotaRequestsCard />}
            </div>
          )}
        </SectionPageLayout.Content>
      </SectionPageLayout>
      {cyclesQuery.data && (
        <CreateCycleDialog
          open={cycleDialogOpen}
          onOpenChange={setCycleDialogOpen}
          defaults={cyclesQuery.data.defaults}
          recommendation={cyclesQuery.data.recommendation}
        />
      )}
    </>
  )
}
