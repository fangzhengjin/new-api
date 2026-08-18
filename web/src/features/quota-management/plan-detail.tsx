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

import { getPlan } from './api'
import { PlanActions } from './components/plan-actions'
import {
  EmptyData,
  PageError,
  PageLoading,
  PlanTypeBadge,
  StatusBadge,
} from './components/shared'
import type { PlanSummary, QuotaCategory } from './types'
import { formatDateTime, formatQuota, queryKeys } from './utils'

function SummaryTable(props: { summary: PlanSummary }) {
  const { t } = useTranslation()
  const rows: Array<[string, QuotaCategory]> = [
    [t('Base increase'), props.summary.base_increase],
    [t('Supplement'), props.summary.supplement],
    [t('Usage bonus'), props.summary.bonus],
    [t('Weighted pool'), props.summary.weighted_pool],
    [t('All increases'), props.summary.increase],
    [t('Ordinary decrease'), props.summary.ordinary_decrease],
    [t('Final low-usage decrease'), props.summary.final_low_decrease],
    [t('Disabled account reclaim'), props.summary.disabled_decrease],
    [t('All decreases'), props.summary.decrease],
  ]
  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>{t('Category')}</TableHead>
          <TableHead>{t('Users')}</TableHead>
          <TableHead className='text-right'>{t('Total')}</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {rows.map(([label, category]) => (
          <TableRow key={label}>
            <TableCell>{label}</TableCell>
            <TableCell>{category.count}</TableCell>
            <TableCell className='text-right'>
              {formatQuota(category.total)}
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  )
}

function FundFlowTable(props: { summary: PlanSummary }) {
  const { t } = useTranslation()
  const rows: Array<[string, string]> = [
    [t('Purchasing budget'), props.summary.budget],
    [t('Current stage cap'), props.summary.stage_cap],
    [t('Total consumed'), props.summary.total_spend],
    [t('Managed balance before'), props.summary.managed_balance],
    [t('Occupied before'), props.summary.occupied_before],
    [t('Occupied after'), props.summary.occupied_after],
    [t('Managed balance after'), props.summary.balance_after],
    [t('Available before reclaim'), props.summary.available_before_reclaim],
    [t('Stage source total'), props.summary.stage_source_total],
    [t('Reclaimed for increases'), props.summary.reclaimed_used_for_increases],
    [t('Reclaimed for overage'), props.summary.reclaimed_used_to_cover_overage],
    [t('Reclaimed unused'), props.summary.reclaimed_unused],
    [t('Original stage unused'), props.summary.stage_original_unused],
    [t('Stage remaining'), props.summary.stage_remaining],
    [t('Pool remaining'), props.summary.pool_remaining],
    [t('Recovery reserve'), props.summary.recovery_reserve],
    [t('Reserved for future stages'), props.summary.future_reserved],
  ]
  return (
    <Table>
      <TableBody>
        {rows.map(([label, value]) => (
          <TableRow key={label}>
            <TableCell>{label}</TableCell>
            <TableCell className='text-right font-medium'>
              {formatQuota(value)}
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
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
  const parameters = plan.parameters

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>
        {t('Quota plan')} #{plan.id}
      </SectionPageLayout.Title>
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
                <PlanTypeBadge
                  type={plan.plan_type}
                  thorough={parameters.thorough_release === true}
                  t={t}
                />
              </CardContent>
            </Card>
            <Card>
              <CardHeader>
                <CardDescription>{t('Stage')}</CardDescription>
                <CardTitle>{plan.stage_percent / 100}%</CardTitle>
              </CardHeader>
              <CardContent>
                {summary.final_stage ? t('Final stage') : t('Dynamic stage')}
              </CardContent>
            </Card>
            <Card>
              <CardHeader>
                <CardDescription>{t('Affected users')}</CardDescription>
                <CardTitle>{summary.affected}</CardTitle>
              </CardHeader>
              <CardContent>
                {formatQuota(plan.planned_delta_quota, true)}
              </CardContent>
            </Card>
            <Card>
              <CardHeader>
                <CardDescription>{t('Snapshot')}</CardDescription>
                <CardTitle>{formatDateTime(plan.snapshot_at)}</CardTitle>
              </CardHeader>
              <CardContent>
                {t('Algorithm')} {plan.algorithm_version}
              </CardContent>
            </Card>
          </div>

          <div className='grid gap-4 lg:grid-cols-2'>
            <Card>
              <CardHeader>
                <CardTitle>{t('Adjustment summary')}</CardTitle>
                <CardDescription>
                  {t('Counts and totals by allocation reason.')}
                </CardDescription>
              </CardHeader>
              <CardContent>
                <SummaryTable summary={summary} />
              </CardContent>
            </Card>
            <Card>
              <CardHeader>
                <CardTitle>{t('Fund flow')}</CardTitle>
                <CardDescription>
                  {t(
                    'Conserved quota sources and destinations for this draft.'
                  )}
                </CardDescription>
              </CardHeader>
              <CardContent>
                <FundFlowTable summary={summary} />
              </CardContent>
            </Card>
          </div>

          <Card>
            <CardHeader>
              <CardTitle>{t('Plan rules')}</CardTitle>
              <CardDescription>
                {t('Immutable inputs saved with this calculation snapshot.')}
              </CardDescription>
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
                  {t('Next adjustment')}
                </div>
                <div className='font-medium'>
                  {formatDateTime(plan.next_adjustment_at)}
                </div>
              </div>
              <div>
                <div className='text-muted-foreground'>{t('Time basis')}</div>
                <div className='font-medium'>
                  {parameters.basis_mode === 'week'
                    ? t('Weekly basis')
                    : t('Actual elapsed days')}
                </div>
              </div>
              <div>
                <div className='text-muted-foreground'>{t('Workdays')}</div>
                <div className='font-medium'>
                  {String(parameters.remaining_workdays ?? '—')} /{' '}
                  {String(parameters.total_workdays ?? '—')}
                </div>
              </div>
              <div>
                <div className='text-muted-foreground'>
                  {t('Early reclaim')}
                </div>
                <div className='font-medium'>
                  {parameters.early_reclaim ? t('Enabled') : t('Disabled')}
                </div>
              </div>
              <div>
                <div className='text-muted-foreground'>{t('Reclaim cap')}</div>
                <div className='font-medium'>
                  {String(parameters.reclaim_cap_percent ?? '—')}%
                </div>
              </div>
              <div>
                <div className='text-muted-foreground'>{t('Usage bonus')}</div>
                <div className='font-medium'>
                  {String(parameters.usage_bonus_percent ?? '—')}%
                </div>
              </div>
              <div>
                <div className='text-muted-foreground'>
                  {t('Thorough release')}
                </div>
                <div className='font-medium'>
                  {parameters.thorough_release ? t('Yes') : t('No')}
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
                  <div className='text-muted-foreground'>{t('Executed')}</div>
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
              <CardTitle>{t('User adjustments')}</CardTitle>
              <CardDescription>
                {t(
                  'Balances, retained amounts, reasons, and notification delivery states.'
                )}
              </CardDescription>
            </CardHeader>
            <CardContent>
              {!items.length ? (
                <EmptyData
                  title={t('No adjustment items')}
                  description={t(
                    'This plan does not contain any user changes.'
                  )}
                />
              ) : (
                <div className='max-h-[34rem] overflow-auto rounded-xl border'>
                  <Table>
                    <TableHeader className='bg-background sticky top-0 z-10'>
                      <TableRow>
                        <TableHead>{t('User')}</TableHead>
                        <TableHead>{t('Action')}</TableHead>
                        <TableHead>{t('Balance at snapshot')}</TableHead>
                        <TableHead>{t('Adjustment')}</TableHead>
                        <TableHead>{t('Retained')}</TableHead>
                        <TableHead>{t('Result')}</TableHead>
                        <TableHead>{t('Log')}</TableHead>
                        <TableHead>{t('Email')}</TableHead>
                        <TableHead className='min-w-72'>{t('Basis')}</TableHead>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {items.map((item) => (
                        <TableRow key={item.id}>
                          <TableCell>
                            <div className='font-medium'>
                              {item.display_name || item.username}
                            </div>
                            <div className='text-muted-foreground'>
                              {item.username} · #{item.user_id}
                            </div>
                          </TableCell>
                          <TableCell>
                            <StatusBadge value={item.action} t={t} />
                          </TableCell>
                          <TableCell>
                            {formatQuota(item.snapshot_balance_quota)}
                          </TableCell>
                          <TableCell
                            className={
                              BigInt(item.adjustment_quota) < 0n
                                ? 'text-destructive'
                                : ''
                            }
                          >
                            {formatQuota(item.adjustment_quota, true)}
                          </TableCell>
                          <TableCell>
                            {formatQuota(item.retained_quota)}
                          </TableCell>
                          <TableCell>
                            {formatQuota(
                              item.actual_before_quota ??
                                item.snapshot_balance_quota
                            )}{' '}
                            →{' '}
                            {formatQuota(
                              item.actual_after_quota ??
                                (
                                  BigInt(item.snapshot_balance_quota) +
                                  BigInt(item.adjustment_quota)
                                ).toString()
                            )}
                          </TableCell>
                          <TableCell title={item.log_error}>
                            <StatusBadge value={item.log_status} t={t} />
                            {item.log_error && (
                              <div className='text-destructive mt-1 max-w-48 whitespace-normal'>
                                {item.log_error}
                              </div>
                            )}
                          </TableCell>
                          <TableCell title={item.email_error}>
                            <StatusBadge value={item.email_status} t={t} />
                            {item.email_error && (
                              <div className='text-destructive mt-1 max-w-48 whitespace-normal'>
                                {item.email_error}
                              </div>
                            )}
                          </TableCell>
                          <TableCell className='whitespace-normal'>
                            {item.basis_text}
                          </TableCell>
                        </TableRow>
                      ))}
                    </TableBody>
                  </Table>
                </div>
              )}
            </CardContent>
          </Card>
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
