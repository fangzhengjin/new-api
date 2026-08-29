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
import { ArrowLeft01Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { Fragment } from 'react'
import { useTranslation } from 'react-i18next'

import { SectionPageLayout } from '@/components/layout'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { cn } from '@/lib/utils'

import { getPlan } from './api'
import { PlanActions } from './components/plan-actions'
import {
  EmptyData,
  PageError,
  PageLoading,
  PlanTypeBadge,
  StatusBadge,
} from './components/shared'
import type { PlanSummary, QuotaItem, QuotaPlan } from './types'
import { formatDateTime, formatQuota, queryKeys } from './utils'

function FundFlow(props: { plan: QuotaPlan; summary: PlanSummary }) {
  const { t } = useTranslation()
  const allocationAfter =
    props.plan.allocation_after_quota ?? props.summary.occupied_after
  const issuedTextClassName =
    '[color:color-mix(in_oklch,var(--primary)_55%,var(--foreground))]'
  const totals = [
    {
      label: t('Cycle total'),
      value: props.summary.budget,
      operator: '',
    },
    {
      label: t('Total issued after this plan'),
      value: allocationAfter,
      operator: '=',
    },
    {
      label: t('Cycle remaining'),
      value: props.summary.pool_remaining,
      operator: '+',
    },
  ]
  const flows = [
    {
      label: t('Issued from cycle remaining quota'),
      value: props.summary.cycle_pool_issued_quota,
      activeClassName: cn('bg-primary/10', issuedTextClassName),
    },
    {
      label: t('Reclaimed and reissued'),
      value: props.summary.reclaimed_used_for_increases,
      activeClassName: cn('bg-primary/10', issuedTextClassName),
    },
    {
      label: t('Reclaimed to cover existing excess'),
      value: props.summary.reclaimed_used_to_cover_overage,
      activeClassName:
        'bg-warning/10 [color:color-mix(in_oklch,var(--warning)_55%,var(--foreground))]',
    },
    {
      label: t('Reclaimed and returned to cycle quota pool'),
      value: props.summary.reclaimed_unused,
      activeClassName:
        'bg-success/10 [color:color-mix(in_oklch,var(--success)_55%,var(--foreground))]',
    },
  ]
  return (
    <div className='grid gap-4'>
      <div className='grid gap-2 md:grid-cols-[minmax(0,1fr)_1.5rem_minmax(0,1fr)_1.5rem_minmax(0,1fr)]'>
        {totals.map((item, index) => (
          <Fragment key={item.label}>
            {index > 0 && (
              <span
                className='bg-background text-muted-foreground mx-auto flex size-6 items-center justify-center rounded-full border text-xs md:self-center'
                aria-hidden='true'
              >
                {item.operator}
              </span>
            )}
            <div className='rounded-lg border p-3'>
              <div className='text-muted-foreground text-xs'>{item.label}</div>
              <div className='mt-1 font-medium tabular-nums'>
                {formatQuota(item.value)}
              </div>
            </div>
          </Fragment>
        ))}
      </div>
      <div className='grid gap-2 border-t pt-4'>
        <div className='flex flex-col gap-1 sm:flex-row sm:items-baseline sm:justify-between'>
          <div className='font-medium'>{t('Plan fund flow')}</div>
          <div className='text-muted-foreground flex flex-wrap gap-x-4 gap-y-1 text-sm'>
            <span>
              {t('Issued in this plan')}{' '}
              <strong
                className={cn('font-medium tabular-nums', issuedTextClassName)}
              >
                {formatQuota(props.summary.increase.total)}
              </strong>
            </span>
            <span>
              {t('Reclaimed from user accounts')}{' '}
              <strong className='text-foreground font-medium tabular-nums'>
                {formatQuota(props.summary.decrease.total)}
              </strong>
            </span>
          </div>
        </div>
        <div className='grid gap-2 sm:grid-cols-2 xl:grid-cols-4'>
          {flows.map((item) => {
            const active = BigInt(item.value) > 0n
            return (
              <div
                key={item.label}
                className={cn(
                  'rounded-lg px-3 py-2.5',
                  active
                    ? item.activeClassName
                    : 'bg-muted/40 text-muted-foreground'
                )}
              >
                <div className='text-xs'>{item.label}</div>
                <div className='mt-1 font-medium tabular-nums'>
                  {formatQuota(item.value)}
                </div>
              </div>
            )
          })}
        </div>
      </div>
    </div>
  )
}

function AdjustmentTable(props: { items: QuotaItem[] }) {
  const { t } = useTranslation()
  return (
    <div className='max-h-[34rem] overflow-auto rounded-xl border'>
      <Table>
        <TableHeader className='bg-background sticky top-0 z-10'>
          <TableRow>
            <TableHead>{t('User')}</TableHead>
            <TableHead>{t('Action')}</TableHead>
            <TableHead className='text-right'>{t('Before')}</TableHead>
            <TableHead className='text-right'>{t('Change')}</TableHead>
            <TableHead className='text-right'>{t('After')}</TableHead>
            <TableHead className='min-w-64'>{t('Reason')}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {props.items.map((item) => {
            const before =
              item.actual_before_quota ?? item.snapshot_balance_quota
            const after =
              item.actual_after_quota ??
              (BigInt(before) + BigInt(item.adjustment_quota)).toString()
            return (
              <TableRow key={item.id}>
                <TableCell>
                  <div className='font-medium'>
                    {item.display_name || item.username}
                  </div>
                  <div className='text-muted-foreground text-xs'>
                    {item.username} · #{item.user_id}
                  </div>
                </TableCell>
                <TableCell>
                  <StatusBadge value={item.action} t={t} />
                </TableCell>
                <TableCell className='text-right tabular-nums'>
                  {formatQuota(before)}
                </TableCell>
                <TableCell
                  className={`text-right font-medium tabular-nums ${BigInt(item.adjustment_quota) < 0n ? 'text-destructive' : ''}`}
                >
                  {formatQuota(item.adjustment_quota, true)}
                </TableCell>
                <TableCell className='text-right tabular-nums'>
                  {formatQuota(after)}
                </TableCell>
                <TableCell className='whitespace-normal'>
                  {item.basis_text}
                </TableCell>
              </TableRow>
            )
          })}
        </TableBody>
      </Table>
      {!props.items.length && (
        <div className='text-muted-foreground p-6 text-center text-sm'>
          {t('No matching adjustments')}
        </div>
      )}
    </div>
  )
}

export function QuotaPlanDetail(props: { planId: number }) {
  const { t } = useTranslation()
  const query = useQuery({
    queryKey: queryKeys.plan(props.planId),
    queryFn: () => getPlan(props.planId),
  })

  if (query.isPending) {
    return (
      <SectionPageLayout>
        <SectionPageLayout.Title>{t('Quota plan')}</SectionPageLayout.Title>
        <SectionPageLayout.Content>
          <PageLoading />
        </SectionPageLayout.Content>
      </SectionPageLayout>
    )
  }
  if (query.isError) {
    return (
      <SectionPageLayout>
        <SectionPageLayout.Title>{t('Quota plan')}</SectionPageLayout.Title>
        <SectionPageLayout.Content>
          <PageError
            title={t('Failed to load quota plan')}
            error={query.error}
          />
        </SectionPageLayout.Content>
      </SectionPageLayout>
    )
  }
  const { plan, cycle, items, summary } = query.data
  const issuedItems = items.filter((item) => BigInt(item.adjustment_quota) > 0n)
  const reclaimedItems = items.filter(
    (item) => BigInt(item.adjustment_quota) < 0n
  )
  const failedNotifications = items.filter(
    (item) => item.log_status === 'failed' || item.email_status === 'failed'
  )
  let defaultDetailTab = 'all'
  if (issuedItems.length) defaultDetailTab = 'issue'
  else if (reclaimedItems.length) defaultDetailTab = 'reclaim'

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>{plan.name}</SectionPageLayout.Title>
      <SectionPageLayout.Breadcrumb>
        <Button
          variant='ghost'
          size='sm'
          className='text-muted-foreground h-auto px-0 py-1 text-xs'
          render={
            <Link
              to='/quota-management/cycles/$cycleId'
              params={{ cycleId: String(plan.cycle_id) }}
            />
          }
        >
          <HugeiconsIcon icon={ArrowLeft01Icon} strokeWidth={2} />
          {t('Back to quota cycle')}
        </Button>
      </SectionPageLayout.Breadcrumb>
      <SectionPageLayout.Actions>
        <PlanActions detail={query.data} />
      </SectionPageLayout.Actions>
      <SectionPageLayout.Content>
        <div className='grid gap-4'>
          {plan.status === 'cancelled' && plan.cancel_reason && (
            <Alert>
              <AlertTitle>{t('Cancellation reason')}</AlertTitle>
              <AlertDescription>{plan.cancel_reason}</AlertDescription>
            </Alert>
          )}
          <div className='grid gap-4 sm:grid-cols-2 lg:grid-cols-4'>
            <Card>
              <CardHeader>
                <CardDescription>{t('Status')}</CardDescription>
                <CardTitle>
                  <StatusBadge value={plan.status} t={t} />
                </CardTitle>
              </CardHeader>
              <CardContent>
                <PlanTypeBadge type={plan.plan_type} t={t} />
              </CardContent>
            </Card>
            <Card>
              <CardHeader>
                <CardDescription>{t('Affected users')}</CardDescription>
                <CardTitle>{summary.affected}</CardTitle>
              </CardHeader>
              <CardContent>
                {t('{{issued}} issued · {{reclaimed}} reclaimed', {
                  issued: summary.increase.count,
                  reclaimed: summary.decrease.count,
                })}
              </CardContent>
            </Card>
            <Card>
              <CardHeader>
                <CardDescription>{t('Planned net change')}</CardDescription>
                <CardTitle>
                  {formatQuota(plan.planned_delta_quota, true)}
                </CardTitle>
              </CardHeader>
              <CardContent>{t('Issue minus reclaim')}</CardContent>
            </Card>
            <Card>
              <CardHeader>
                <CardDescription>{t('Calculated at')}</CardDescription>
                <CardTitle>{formatDateTime(plan.snapshot_at)}</CardTitle>
              </CardHeader>
              <CardContent>
                {t('Next planned adjustment')}:{' '}
                {formatDateTime(plan.next_adjustment_at)}
              </CardContent>
            </Card>
          </div>

          <Card>
            <CardHeader>
              <CardTitle>{t('Quota flow')}</CardTitle>
              <CardDescription>
                {t(
                  "Shows how the cycle total is allocated, plus where this plan's issued and reclaimed quota went"
                )}
              </CardDescription>
            </CardHeader>
            <CardContent>
              <FundFlow plan={plan} summary={summary} />
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle>{t('Plan information')}</CardTitle>
              <CardDescription>{plan.purpose}</CardDescription>
            </CardHeader>
            <CardContent className='grid gap-3 sm:grid-cols-2 lg:grid-cols-4'>
              <div>
                <div className='text-muted-foreground'>{t('Cycle')}</div>
                <div className='font-medium'>
                  #{cycle.id} · {formatDateTime(cycle.cycle_start_at)} –{' '}
                  {formatDateTime(cycle.cycle_end_at)}
                </div>
              </div>
              <div>
                <div className='text-muted-foreground'>
                  {t('Next planned adjustment')}
                </div>
                <div className='font-medium'>
                  {formatDateTime(plan.next_adjustment_at)}
                </div>
              </div>
              <div>
                <div className='text-muted-foreground'>{t('Created at')}</div>
                <div className='font-medium'>
                  {formatDateTime(plan.created_at)}
                </div>
              </div>
              {plan.executed_at && (
                <div>
                  <div className='text-muted-foreground'>
                    {t('Executed at')}
                  </div>
                  <div className='font-medium'>
                    {formatDateTime(plan.executed_at)}
                  </div>
                </div>
              )}
              {plan.cancelled_at && (
                <div>
                  <div className='text-muted-foreground'>
                    {t('Cancelled at')}
                  </div>
                  <div className='font-medium'>
                    {formatDateTime(plan.cancelled_at)}
                  </div>
                </div>
              )}
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle>{t('Allocation details')}</CardTitle>
              <CardDescription>
                {t(
                  'Review who receives quota, who returns quota, and the balance before and after each change'
                )}
              </CardDescription>
            </CardHeader>
            <CardContent>
              {!items.length ? (
                <EmptyData
                  title={t('No adjustment items')}
                  description={t('This plan does not contain any user changes')}
                />
              ) : (
                <Tabs defaultValue={defaultDetailTab}>
                  <TabsList aria-label={t('Filter allocation details')}>
                    <TabsTrigger value='issue'>
                      {t('Issue')} ({issuedItems.length})
                    </TabsTrigger>
                    <TabsTrigger value='reclaim'>
                      {t('Reclaim')} ({reclaimedItems.length})
                    </TabsTrigger>
                    <TabsTrigger value='all'>
                      {t('All')} ({items.length})
                    </TabsTrigger>
                  </TabsList>
                  <TabsContent value='issue'>
                    <AdjustmentTable items={issuedItems} />
                  </TabsContent>
                  <TabsContent value='reclaim'>
                    <AdjustmentTable items={reclaimedItems} />
                  </TabsContent>
                  <TabsContent value='all'>
                    <AdjustmentTable items={items} />
                  </TabsContent>
                </Tabs>
              )}
            </CardContent>
          </Card>
          {failedNotifications.length > 0 && (
            <Alert variant='destructive'>
              <AlertTitle>{t('Some notifications need retry')}</AlertTitle>
              <AlertDescription>
                {t(
                  '{{count}} user notifications failed after quota changes were completed',
                  {
                    count: failedNotifications.length,
                  }
                )}
              </AlertDescription>
            </Alert>
          )}
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
