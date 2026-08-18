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
import { useTranslation } from 'react-i18next'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'

import type { FairnessMetrics, FairnessShadowComparison } from '../types'
import { formatDateTime, formatQuota } from '../utils'

function formatCoverage(basisPoints: number): string {
  return `${new Intl.NumberFormat(undefined, { maximumFractionDigits: 2 }).format(basisPoints / 100)}%`
}

export function FairnessShadowResult(props: {
  result: FairnessShadowComparison
}) {
  const { t } = useTranslation()
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

  return (
    <div className='grid gap-4' aria-live='polite'>
      <Alert
        variant={props.result.candidate_qualified ? 'default' : 'destructive'}
      >
        <AlertTitle>
          {props.result.candidate_qualified
            ? t('Candidate passes shadow gate')
            : t('Candidate does not pass shadow gate')}
        </AlertTitle>
        <AlertDescription>
          {t('Read-only comparison; no draft or balance changes were made.')}
          <br />
          {t('Snapshot')}: {formatDateTime(props.result.snapshot_at)} ·{' '}
          {t('Stage cap')}: {formatQuota(props.result.stage_cap_quota)} ·{' '}
          {t('Recovery reserve')}:{' '}
          {formatQuota(props.result.current.recovery_reserve_quota)}
        </AlertDescription>
      </Alert>

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

      {!!props.result.items.length && (
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
      )}
    </div>
  )
}
