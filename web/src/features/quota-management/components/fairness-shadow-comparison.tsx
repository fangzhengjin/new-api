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
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'

import type {
  ConcentrationShadowVariant,
  FairnessMetrics,
  FairnessShadowComparison,
} from '../types'
import {
  formatConcentrationMultiplier,
  formatDateTime,
  formatQuota,
} from '../utils'

function formatCoverage(basisPoints: number): string {
  return `${new Intl.NumberFormat(undefined, { maximumFractionDigits: 2 }).format(basisPoints / 100)}%`
}

export function FairnessShadowResult(props: {
  result: FairnessShadowComparison
  selectedMultiplierBasisPoints?: number
}) {
  const { t } = useTranslation()
  const configuredVariant = props.result.concentration_variants.find(
    (variant) =>
      variant.multiplier_basis_points === props.selectedMultiplierBasisPoints
  )
  const [selectedMultiplier, setSelectedMultiplier] = useState(() =>
    String(
      configuredVariant?.multiplier_basis_points ??
        props.result.concentration_variants[0]?.multiplier_basis_points ??
        ''
    )
  )
  const [search, setSearch] = useState('')
  const [pageSize, setPageSize] = useState(50)
  const [page, setPage] = useState(1)
  const selectedVariant = props.result.concentration_variants.find(
    (variant) => String(variant.multiplier_basis_points) === selectedMultiplier
  )
  const filteredItems = useMemo(() => {
    if (!selectedVariant) return []
    const query = search.trim().toLocaleLowerCase()
    if (!query) return selectedVariant.items
    return selectedVariant.items.filter(
      (item) =>
        item.username.toLocaleLowerCase().includes(query) ||
        String(item.user_id).includes(query)
    )
  }, [search, selectedVariant])
  const pageCount = Math.max(1, Math.ceil(filteredItems.length / pageSize))
  const currentPage = Math.min(page, pageCount)
  const pageStart = (currentPage - 1) * pageSize
  const visibleItems = filteredItems.slice(pageStart, pageStart + pageSize)
  const usesCycleLimit = configuredVariant !== undefined
  let alertTitle = t('Preview')
  if (!usesCycleLimit) {
    alertTitle = props.result.candidate_qualified
      ? t('Candidate passes shadow gate')
      : t('Candidate does not pass shadow gate')
  }
  const metricRows: Array<
    [string, (metrics: FairnessMetrics) => string | number]
  > = [
    [t('Eligible users'), (metrics) => metrics.population],
    [
      t('Minimum coverage'),
      (metrics) => formatCoverage(metrics.minimum_coverage_basis_points),
    ],
    [
      t('P10 coverage'),
      (metrics) => formatCoverage(metrics.p10_coverage_basis_points),
    ],
    [
      t('P50 coverage'),
      (metrics) => formatCoverage(metrics.p50_coverage_basis_points),
    ],
    [
      t('P90 coverage'),
      (metrics) => formatCoverage(metrics.p90_coverage_basis_points),
    ],
    [
      t('Minimum safety coverage'),
      (metrics) => formatCoverage(metrics.minimum_safety_coverage_basis_points),
    ],
    [t('Safety unmet users'), (metrics) => metrics.safety_unmet],
    [
      t('New user coverage'),
      (metrics) =>
        metrics.new_user_coverage_basis_points === null
          ? '—'
          : formatCoverage(metrics.new_user_coverage_basis_points),
    ],
    [t('Reclaimed'), (metrics) => formatQuota(metrics.reclaimed_quota)],
    [
      t('Occupied after'),
      (metrics) => formatQuota(metrics.occupied_after_quota),
    ],
  ]
  const concentrationMetricRows: Array<
    [string, (variant: ConcentrationShadowVariant) => string | number]
  > = [
    [t('Eligible users'), (variant) => variant.population],
    [
      t('Position ceiling'),
      (variant) => formatQuota(variant.position_ceiling_quota),
    ],
    [
      t('Maximum position share'),
      (variant) => formatCoverage(variant.maximum_position_share_basis_points),
    ],
    [t('Capped users'), (variant) => variant.capped_users],
    [t('Capped quota'), (variant) => formatQuota(variant.capped_quota)],
    [
      t('Minimum raw demand coverage'),
      (variant) => formatCoverage(variant.minimum_raw_coverage_basis_points),
    ],
    [
      t('P10 raw demand coverage'),
      (variant) => formatCoverage(variant.p10_raw_coverage_basis_points),
    ],
    [
      t('P50 raw demand coverage'),
      (variant) => formatCoverage(variant.p50_raw_coverage_basis_points),
    ],
    [
      t('Minimum effective coverage'),
      (variant) =>
        formatCoverage(variant.minimum_effective_coverage_basis_points),
    ],
    [
      t('P10 effective coverage'),
      (variant) => formatCoverage(variant.p10_effective_coverage_basis_points),
    ],
    [
      t('P50 effective coverage'),
      (variant) => formatCoverage(variant.p50_effective_coverage_basis_points),
    ],
    [
      t('Minimum safety coverage'),
      (variant) => formatCoverage(variant.minimum_safety_coverage_basis_points),
    ],
    [t('Safety unmet users'), (variant) => variant.safety_unmet],
    [
      t('Planned increase'),
      (variant) => formatQuota(variant.planned_increase_quota),
    ],
    [
      t('Unallocated stage capacity'),
      (variant) => formatQuota(variant.unallocated_stage_quota),
    ],
  ]

  return (
    <div className='grid gap-4' aria-live='polite'>
      <Alert
        variant={
          !usesCycleLimit && !props.result.candidate_qualified
            ? 'destructive'
            : 'default'
        }
      >
        <AlertTitle>{alertTitle}</AlertTitle>
        <AlertDescription>
          {t('Read-only comparison; no draft or balance changes were made.')}
          <br />
          {t('Snapshot')}: {formatDateTime(props.result.snapshot_at)} ·{' '}
          {t('Stage cap')}: {formatQuota(props.result.stage_cap_quota)} ·{' '}
          {t('Recovery reserve')}:{' '}
          {formatQuota(props.result.current.recovery_reserve_quota)}
          <br />
          {usesCycleLimit
            ? t(
                'This cycle uses {{multiplier}} for regular automatic adjustments. Previewing another tab does not change the cycle setting.',
                {
                  multiplier: formatConcentrationMultiplier(
                    configuredVariant?.multiplier_basis_points ?? 0
                  ),
                }
              )
            : t(
                'The 1.5×, 2×, and 3× concentration projections are for selection only and do not affect the production switch gate.'
              )}
        </AlertDescription>
      </Alert>

      {!usesCycleLimit && (
        <div className='overflow-auto rounded-xl border'>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t('Metric')}</TableHead>
                <TableHead>
                  {t('Current algorithm')} ·{' '}
                  {props.result.current_algorithm_version}
                </TableHead>
                <TableHead>
                  {t('Candidate algorithm')} ·{' '}
                  {props.result.candidate_algorithm_version}
                </TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {metricRows.map(([label, value]) => (
                <TableRow key={label}>
                  <TableCell>{label}</TableCell>
                  <TableCell>{value(props.result.current)}</TableCell>
                  <TableCell>{value(props.result.candidate)}</TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      )}

      {selectedVariant ? (
        <section className='grid gap-2' aria-labelledby='concentration-summary'>
          <div>
            <h3 id='concentration-summary' className='font-medium'>
              {t('Cumulative position concentration comparison')}
            </h3>
            <p className='text-muted-foreground text-sm'>
              {t(
                'Raw coverage uses uncapped demand; effective coverage uses the demand allowed after the cumulative position ceiling.'
              )}
            </p>
          </div>
          <Tabs
            value={selectedMultiplier}
            onValueChange={(value) => {
              setSelectedMultiplier(value)
              setPage(1)
            }}
          >
            <TabsList aria-label={t('Concentration multiplier comparison')}>
              {props.result.concentration_variants.map((variant) => (
                <TabsTrigger
                  key={variant.multiplier_basis_points}
                  value={String(variant.multiplier_basis_points)}
                >
                  {formatConcentrationMultiplier(
                    variant.multiplier_basis_points
                  )}
                  {variant === configuredVariant
                    ? ` · ${t('Cycle setting')}`
                    : ''}
                </TabsTrigger>
              ))}
            </TabsList>
            <TabsContent value={selectedMultiplier} className='grid gap-4'>
              <div className='overflow-auto rounded-xl border'>
                <Table>
                  <caption className='sr-only'>
                    {t('Concentration multiplier comparison')}
                  </caption>
                  <TableHeader>
                    <TableRow>
                      <TableHead>{t('Metric')}</TableHead>
                      <TableHead>{t('Result')}</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {concentrationMetricRows.map(([label, value]) => (
                      <TableRow key={label}>
                        <TableCell>{label}</TableCell>
                        <TableCell>{value(selectedVariant)}</TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </div>

              <section
                className='grid gap-2'
                aria-labelledby='concentration-details'
              >
                <div>
                  <h3 id='concentration-details' className='font-medium'>
                    {t('User cumulative position details')}
                  </h3>
                  <p className='text-muted-foreground text-sm'>
                    {t(
                      'Period spend comes from cycle settlements and includes the balance already consumed before this snapshot.'
                    )}
                  </p>
                </div>
                <div className='flex flex-col gap-2 sm:flex-row sm:items-end sm:justify-between'>
                  <div className='w-full sm:max-w-xs'>
                    <label
                      className='sr-only'
                      htmlFor='concentration-user-search'
                    >
                      {t('Search')}
                    </label>
                    <Input
                      id='concentration-user-search'
                      value={search}
                      placeholder={t('Search by username or user ID')}
                      onChange={(event) => {
                        setSearch(event.target.value)
                        setPage(1)
                      }}
                    />
                  </div>
                  <div className='flex items-center gap-2'>
                    <label
                      className='text-muted-foreground text-sm'
                      htmlFor='concentration-page-size'
                    >
                      {t('Rows per page')}
                    </label>
                    <NativeSelect
                      id='concentration-page-size'
                      value={String(pageSize)}
                      onChange={(event) => {
                        setPageSize(Number(event.target.value))
                        setPage(1)
                      }}
                    >
                      {[20, 50, 100].map((size) => (
                        <NativeSelectOption key={size} value={size}>
                          {size}
                        </NativeSelectOption>
                      ))}
                    </NativeSelect>
                  </div>
                </div>
                <div className='max-h-80 overflow-auto rounded-xl border'>
                  <Table>
                    <caption className='sr-only'>
                      {t('Per-user concentration projection details')}
                    </caption>
                    <TableHeader className='bg-background sticky top-0 z-10'>
                      <TableRow>
                        <TableHead>{t('User')}</TableHead>
                        <TableHead>{t('Period spend')}</TableHead>
                        <TableHead>{t('Spend share')}</TableHead>
                        <TableHead>{t('Current balance')}</TableHead>
                        <TableHead>{t('Current position')}</TableHead>
                        <TableHead>{t('Position ceiling')}</TableHead>
                        <TableHead>{t('Quota change')}</TableHead>
                        <TableHead>{t('Capped quota')}</TableHead>
                        <TableHead>{t('Position after')}</TableHead>
                        <TableHead>{t('Raw demand coverage')}</TableHead>
                        <TableHead>{t('Effective coverage')}</TableHead>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {visibleItems.length > 0 ? (
                        visibleItems.map((item) => (
                          <TableRow key={item.user_id}>
                            <TableCell>
                              <div className='font-medium'>{item.username}</div>
                              <div className='text-muted-foreground'>
                                #{item.user_id}
                              </div>
                            </TableCell>
                            <TableCell>
                              {formatQuota(item.period_spend_quota)}
                            </TableCell>
                            <TableCell>
                              {formatCoverage(item.spend_share_basis_points)}
                            </TableCell>
                            <TableCell>
                              {formatQuota(item.current_balance_quota)}
                            </TableCell>
                            <TableCell>
                              {formatQuota(item.current_position_quota)}
                            </TableCell>
                            <TableCell>
                              {formatQuota(item.position_ceiling_quota)}
                            </TableCell>
                            <TableCell>
                              {formatQuota(item.adjustment_quota, true)}
                            </TableCell>
                            <TableCell>
                              {formatQuota(item.capped_quota)}
                            </TableCell>
                            <TableCell>
                              {formatQuota(item.after_position_quota)}
                            </TableCell>
                            <TableCell>
                              {formatCoverage(item.raw_coverage_basis_points)}
                            </TableCell>
                            <TableCell>
                              {formatCoverage(
                                item.effective_coverage_basis_points
                              )}
                            </TableCell>
                          </TableRow>
                        ))
                      ) : (
                        <TableRow>
                          <TableCell colSpan={11} className='text-center'>
                            {t('No matching results')}
                          </TableCell>
                        </TableRow>
                      )}
                    </TableBody>
                  </Table>
                </div>
                <div className='flex flex-col gap-2 text-sm sm:flex-row sm:items-center sm:justify-between'>
                  <div className='text-muted-foreground'>
                    {t('Showing')} {filteredItems.length ? pageStart + 1 : 0}–
                    {Math.min(pageStart + pageSize, filteredItems.length)} /{' '}
                    {filteredItems.length} {t('users')}
                  </div>
                  <div className='flex items-center gap-2'>
                    <Button
                      type='button'
                      size='sm'
                      variant='outline'
                      disabled={currentPage <= 1}
                      onClick={() => setPage((value) => Math.max(1, value - 1))}
                    >
                      {t('Previous page')}
                    </Button>
                    <span>
                      {t('Page {{current}} of {{total}}', {
                        current: currentPage,
                        total: pageCount,
                      })}
                    </span>
                    <Button
                      type='button'
                      size='sm'
                      variant='outline'
                      disabled={currentPage >= pageCount}
                      onClick={() =>
                        setPage((value) => Math.min(pageCount, value + 1))
                      }
                    >
                      {t('Next page')}
                    </Button>
                  </div>
                </div>
              </section>
            </TabsContent>
          </Tabs>
        </section>
      ) : null}

      {!usesCycleLimit && props.result.items.length > 0 ? (
        <div className='max-h-80 overflow-auto rounded-xl border'>
          <Table>
            <TableHeader className='bg-background sticky top-0 z-10'>
              <TableRow>
                <TableHead>{t('User')}</TableHead>
                <TableHead>{t('Current balance')}</TableHead>
                <TableHead>{t('Target')}</TableHead>
                <TableHead>{t('Current adjustment')}</TableHead>
                <TableHead>{t('Candidate adjustment')}</TableHead>
                <TableHead>{t('Current coverage')}</TableHead>
                <TableHead>{t('Candidate coverage')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {props.result.items.map((item) => (
                <TableRow key={item.user_id}>
                  <TableCell>
                    <div className='font-medium'>{item.username}</div>
                    <div className='text-muted-foreground'>#{item.user_id}</div>
                  </TableCell>
                  <TableCell>
                    {formatQuota(item.current_balance_quota)}
                  </TableCell>
                  <TableCell>{formatQuota(item.target_quota)}</TableCell>
                  <TableCell>
                    {formatQuota(item.current_adjustment_quota, true)}
                  </TableCell>
                  <TableCell>
                    {formatQuota(item.candidate_adjustment_quota, true)}
                  </TableCell>
                  <TableCell>
                    {formatCoverage(item.current_coverage_basis_points)}
                  </TableCell>
                  <TableCell>
                    {formatCoverage(item.candidate_coverage_basis_points)}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      ) : null}
    </div>
  )
}
