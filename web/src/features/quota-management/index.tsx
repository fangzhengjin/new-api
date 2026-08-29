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
import { ActivityCard } from './components/activity-card'
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
              {t('Cycle')} #{props.cycle.id}
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
              {t('Create next cycle')}
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
                aria-label={t('Data update time')}
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
                {t('Updated at {{time}}', {
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
              aria-label={t('Refresh cycle remaining quota')}
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
        title={t('No active cycle')}
        description={t('Create a cycle to start managing quota allocation')}
      />
    )
  }
  if (cycle.allocation_error) {
    return (
      <EmptyData
        title={t('Allocation data error')}
        description={t(
          'The saved allocation data is invalid. Contact a system maintainer before creating or executing plans. User API usage remains available from each user balance'
        )}
      />
    )
  }
  if (cycle.allocated_quota == null || cycle.remaining_quota == null) {
    return (
      <EmptyData
        title={t('Allocation data setup required')}
        description={t(
          'This cycle requires a one-time allocation data setup by a system maintainer before plans can be created or executed. API usage remains available from each user balance'
        )}
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
      label: t('Issued in this plan'),
      value: summary?.increase.total ?? '0',
    },
    {
      label: t('Reclaimed in this plan'),
      value: summary?.decrease.total ?? '0',
    },
    {
      label: t('Reissued from reclaimed quota'),
      value: summary?.reclaimed_used_for_increases ?? '0',
    },
    {
      label: t('Reclaimed to cover existing excess'),
      value: summary?.reclaimed_used_to_cover_overage ?? '0',
    },
    {
      label: t('Issued from cycle remaining quota'),
      value: summary?.cycle_pool_issued_quota ?? '0',
    },
    {
      label: t('Reclaimed and returned to cycle quota pool'),
      value: summary?.reclaimed_unused ?? '0',
    },
    {
      label: t('Cycle remaining quota after this plan'),
      value: summary?.pool_remaining ?? cycle.remaining_quota,
    },
  ]
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('Quota flow')}</CardTitle>
        <CardDescription>
          {t(
            'See how cycle quota is issued, reclaimed, reissued, and retained'
          )}
        </CardDescription>
      </CardHeader>
      <CardContent className='space-y-5'>
        {props.overview.current_plan && (
          <div className='flex flex-col justify-between gap-3 rounded-lg border p-3 sm:flex-row sm:items-center'>
            <div className='min-w-0'>
              <div className='text-muted-foreground text-xs'>
                {t('Current plan')}
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
            <span>{t('Cycle total')}</span>
            <span className='font-medium tabular-nums'>
              {formatQuota(cycle.budget_quota)}
            </span>
          </div>
          <Progress
            value={allocationRate}
            aria-label={t('Allocation progress')}
          />
          <div className='text-muted-foreground mt-2 flex justify-between gap-4 text-xs'>
            <span>
              {t('Allocated')} {formatQuota(cycle.allocated_quota ?? '0')}
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
            {t('View current plan flow')}
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
        title={t('Failed to load quota cycles')}
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
            <span className='truncate'>{t('Cycle Quota Management')}</span>
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
            {t('Create cycle')}
          </Button>
        </SectionPageLayout.Actions>
        <SectionPageLayout.Content>
          {overviewQuery.isPending && <PageLoading />}
          {overviewQuery.isError && (
            <PageError
              title={t('Failed to load quota overview')}
              error={overviewQuery.error}
            />
          )}
          {overviewQuery.data && (
            <div className='space-y-6'>
              <section aria-labelledby='cycle-navigation-title'>
                <div className='mb-3 flex items-center justify-between gap-3'>
                  <div>
                    <h2 id='cycle-navigation-title' className='font-medium'>
                      {t('Cycle overview')}
                    </h2>
                    <p className='text-muted-foreground text-sm'>
                      {t(
                        'Cycles define the valid time range and plans carry out the actual quota allocation'
                      )}
                    </p>
                  </div>
                </div>
                <div className='grid gap-3 md:grid-cols-3'>
                  <CycleSlot
                    label={t('Previous cycle')}
                    cycle={overviewQuery.data.previous_cycle}
                    emptyText={t('No previous cycle')}
                  />
                  <CycleSlot
                    label={t('Current cycle')}
                    cycle={overviewQuery.data.current_cycle}
                    emptyText={t('No active cycle')}
                  />
                  <CycleSlot
                    label={t('Next cycle')}
                    cycle={overviewQuery.data.next_cycle}
                    emptyText={t('Not created yet')}
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
                  <p className='font-medium'>{t('Allocation data error')}</p>
                  <p className='mt-1 text-sm'>
                    {t(
                      'The saved allocation data is invalid. Contact a system maintainer before creating or executing plans. User API usage remains available from each user balance'
                    )}
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
                      {t('Allocation data setup required')}
                    </p>
                    <p className='mt-1 text-sm'>
                      {t(
                        'This cycle requires a one-time allocation data setup by a system maintainer before plans can be created or executed. API usage remains available from each user balance'
                      )}
                    </p>
                  </div>
                )}

              <section aria-labelledby='allocation-summary-title'>
                <div className='mb-3 flex items-center justify-between gap-3'>
                  <h2 id='allocation-summary-title' className='font-medium'>
                    {t('Allocation summary')}
                  </h2>
                  {pendingTotal > 0 && (
                    <Badge variant='outline'>
                      {t('{{count}} pending items', { count: pendingTotal })}
                    </Badge>
                  )}
                </div>
                <div className='grid gap-3 md:grid-cols-3'>
                  <MetricCard
                    label={t('Cycle total')}
                    value={overviewQuery.data.current_cycle?.budget_quota}
                    description={t(
                      'Total quota available in the current cycle'
                    )}
                  />
                  <MetricCard
                    label={t('Allocated quota')}
                    value={overviewQuery.data.current_cycle?.allocated_quota}
                    description={t(
                      'Quota issued and not reclaimed in this cycle'
                    )}
                  />
                  <MetricCard
                    label={t('Cycle remaining')}
                    value={overviewQuery.data.current_cycle?.remaining_quota}
                    description={t(
                      'Quota still available for future plans and temporary quota requests'
                    )}
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
                    <CardTitle>{t('Items requiring attention')}</CardTitle>
                    <CardDescription>
                      {t(
                        'Only items that may affect the next allocation are shown'
                      )}
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
                        t('Users awaiting allocation'),
                        overviewQuery.data.pending_allocation_count,
                      ],
                      [
                        t('Balances awaiting reclaim'),
                        overviewQuery.data.pending_reclaim_count,
                      ],
                      [
                        t('Temporary quota requests'),
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
                  {t('All cycles')}
                </h2>
                {cyclesContent}
              </section>

              {cycleQuotaManagementEnabled && <TemporaryQuotaRequestsCard />}
              <ActivityCard />
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
