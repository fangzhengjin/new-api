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
      label: t('周期总额度'),
      value: props.summary.budget,
      operator: '',
    },
    {
      label: t('本方案后累计已发放'),
      value: allocationAfter,
      operator: '=',
    },
    {
      label: t('周期剩余额度'),
      value: props.summary.pool_remaining,
      operator: '+',
    },
  ]
  const flows = [
    {
      label: t('使用周期剩余额度发放'),
      value: props.summary.cycle_pool_issued_quota,
      activeClassName: cn('bg-primary/10', issuedTextClassName),
    },
    {
      label: t('已回收并重新发放'),
      value: props.summary.reclaimed_used_for_increases,
      activeClassName: cn('bg-primary/10', issuedTextClassName),
    },
    {
      label: t('收回用于冲抵原有超额'),
      value: props.summary.reclaimed_used_to_cover_overage,
      activeClassName:
        'bg-warning/10 [color:color-mix(in_oklch,var(--warning)_55%,var(--foreground))]',
    },
    {
      label: t('收回额度返还周期额度池'),
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
          <div className='font-medium'>{t('本方案资金流')}</div>
          <div className='text-muted-foreground flex flex-wrap gap-x-4 gap-y-1 text-sm'>
            <span>
              {t('本方案发放')}{' '}
              <strong
                className={cn('font-medium tabular-nums', issuedTextClassName)}
              >
                {formatQuota(props.summary.increase.total)}
              </strong>
            </span>
            <span>
              {t('从用户账户收回')}{' '}
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
            <TableHead className='text-right'>{t('调整前')}</TableHead>
            <TableHead className='text-right'>{t('Change')}</TableHead>
            <TableHead className='text-right'>{t('调整后')}</TableHead>
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
          {t('没有匹配的调整记录')}
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
        <SectionPageLayout.Title>{t('额度调配方案')}</SectionPageLayout.Title>
        <SectionPageLayout.Content>
          <PageLoading />
        </SectionPageLayout.Content>
      </SectionPageLayout>
    )
  }
  if (query.isError) {
    return (
      <SectionPageLayout>
        <SectionPageLayout.Title>{t('额度调配方案')}</SectionPageLayout.Title>
        <SectionPageLayout.Content>
          <PageError
            title={t('加载额度方案失败')}
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
          {t('返回额度周期')}
        </Button>
      </SectionPageLayout.Breadcrumb>
      <SectionPageLayout.Actions>
        <PlanActions detail={query.data} />
      </SectionPageLayout.Actions>
      <SectionPageLayout.Content>
        <div className='grid gap-4'>
          {plan.status === 'cancelled' && plan.cancel_reason && (
            <Alert>
              <AlertTitle>{t('取消原因')}</AlertTitle>
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
                <CardDescription>{t('会调整的用户数')}</CardDescription>
                <CardTitle>{summary.affected}</CardTitle>
              </CardHeader>
              <CardContent>
                {t('发放 {{issued}} · 回收 {{reclaimed}}', {
                  issued: summary.increase.count,
                  reclaimed: summary.decrease.count,
                })}
              </CardContent>
            </Card>
            <Card>
              <CardHeader>
                <CardDescription>{t('方案净变动')}</CardDescription>
                <CardTitle>
                  {formatQuota(plan.planned_delta_quota, true)}
                </CardTitle>
              </CardHeader>
              <CardContent>{t('发放减去回收')}</CardContent>
            </Card>
            <Card>
              <CardHeader>
                <CardDescription>{t('计算时间')}</CardDescription>
                <CardTitle>{formatDateTime(plan.snapshot_at)}</CardTitle>
              </CardHeader>
              <CardContent>
                {t('下次方案调整')}:{' '}
                {formatDateTime(plan.next_adjustment_at)}
              </CardContent>
            </Card>
          </div>

          <Card>
            <CardHeader>
              <CardTitle>{t('额度流向')}</CardTitle>
              <CardDescription>
                {t(
                  "展示周期总额的分配结果，以及本方案发放来源和回收去向"
                )}
              </CardDescription>
            </CardHeader>
            <CardContent>
              <FundFlow plan={plan} summary={summary} />
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle>{t('方案信息')}</CardTitle>
              <CardDescription>{plan.purpose}</CardDescription>
            </CardHeader>
            <CardContent className='grid gap-3 sm:grid-cols-2 lg:grid-cols-4'>
              <div>
                <div className='text-muted-foreground'>{t('周期')}</div>
                <div className='font-medium'>
                  #{cycle.id} · {formatDateTime(cycle.cycle_start_at)} –{' '}
                  {formatDateTime(cycle.cycle_end_at)}
                </div>
              </div>
              <div>
                <div className='text-muted-foreground'>
                  {t('下次方案调整')}
                </div>
                <div className='font-medium'>
                  {formatDateTime(plan.next_adjustment_at)}
                </div>
              </div>
              <div>
                <div className='text-muted-foreground'>{t('创建时间')}</div>
                <div className='font-medium'>
                  {formatDateTime(plan.created_at)}
                </div>
              </div>
              {plan.executed_at && (
                <div>
                  <div className='text-muted-foreground'>
                    {t('执行时间')}
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
              <CardTitle>{t('分配明细')}</CardTitle>
              <CardDescription>
                {t('审查获得额度的用户、归还额度的用户，以及每次变更前后的余额')}
              </CardDescription>
            </CardHeader>
            <CardContent>
              {!items.length ? (
                <EmptyData
                  title={t('没有需要调整的用户')}
                  description={t('这份草稿不会修改任何用户余额')}
                />
              ) : (
                <Tabs defaultValue={defaultDetailTab}>
                  <TabsList aria-label={t('筛选分配明细')}>
                    <TabsTrigger value='issue'>
                      {t('发放')} ({issuedItems.length})
                    </TabsTrigger>
                    <TabsTrigger value='reclaim'>
                      {t('回收')} ({reclaimedItems.length})
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
              <AlertTitle>{t('部分通知需要重试')}</AlertTitle>
              <AlertDescription>
                {t('额度变更完成后，{{count}} 条用户通知发送失败',
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
