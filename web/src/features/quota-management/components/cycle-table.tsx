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
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'

import type { QuotaCycle } from '../types'
import { formatDateTime, formatQuota } from '../utils'
import { EmptyData, StatusBadge } from './shared'

export function CycleTable(props: { cycles: QuotaCycle[] }) {
  const { t } = useTranslation()
  if (!props.cycles.length) {
    return (
      <EmptyData
        title={t('No quota cycles')}
        description={t('Create a purchasing cycle to start quota allocation.')}
      />
    )
  }
  return (
    <div className='rounded-xl border'>
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>{t('Cycle')}</TableHead>
            <TableHead>{t('Status')}</TableHead>
            <TableHead>{t('Budget')}</TableHead>
            <TableHead>{t('Initial grant per user')}</TableHead>
            <TableHead>{t('Created by')}</TableHead>
            <TableHead className='text-right'>{t('Actions')}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {props.cycles.map((cycle) => (
            <TableRow key={cycle.id}>
              <TableCell>
                <div className='font-medium'>#{cycle.id}</div>
                <div className='text-muted-foreground whitespace-nowrap'>
                  {formatDateTime(cycle.cycle_start_at)} –{' '}
                  {formatDateTime(cycle.cycle_end_at)}
                </div>
              </TableCell>
              <TableCell>
                <StatusBadge value={cycle.status} t={t} />
              </TableCell>
              <TableCell>{formatQuota(cycle.budget_quota)}</TableCell>
              <TableCell>{formatQuota(cycle.initial_grant_quota)}</TableCell>
              <TableCell>{cycle.created_by || '—'}</TableCell>
              <TableCell className='text-right'>
                <Button
                  size='sm'
                  variant='outline'
                  render={
                    <Link
                      to='/quota-management/cycles/$cycleId'
                      params={{ cycleId: String(cycle.id) }}
                    />
                  }
                >
                  {t('View details')}
                </Button>
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </div>
  )
}
