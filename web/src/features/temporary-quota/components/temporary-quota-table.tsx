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
import { useQuery } from '@tanstack/react-query'
import { getRouteApi } from '@tanstack/react-router'
import type { ColumnDef } from '@tanstack/react-table'
import { Eye } from 'lucide-react'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { DataTablePage, useDataTable } from '@/components/data-table'
import { StatusBadge } from '@/components/status-badge'
import {
  Alert,
  AlertAction,
  AlertDescription,
  AlertTitle,
} from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { useTableUrlState } from '@/hooks/use-table-url-state'
import { getCurrencyLabel } from '@/lib/currency'
import { getServerErrorMessage } from '@/lib/handle-server-error'

import {
  formatDateTime,
  quotaToDisplayAmount,
} from '../../quota-management/utils'
import { listSelfTemporaryQuotaRequests, temporaryQuotaQueryKeys } from '../api'
import {
  TEMPORARY_QUOTA_REQUEST_STATUSES,
  TEMPORARY_QUOTA_REQUEST_STATUS_OPTIONS,
} from '../constants'
import type { SelfTemporaryQuotaRequest } from '../types'

const route = getRouteApi('/_authenticated/temporary-quota/')

export function TemporaryQuotaTable(props: {
  onView: (request: SelfTemporaryQuotaRequest) => void
}) {
  const { t } = useTranslation()
  const currencyLabel = getCurrencyLabel()
  const onView = props.onView
  const columns = useMemo<ColumnDef<SelfTemporaryQuotaRequest>[]>(
    () => [
      {
        accessorKey: 'created_at',
        header: t('Application time'),
        cell: ({ row }) => formatDateTime(row.original.created_at),
        size: 190,
        meta: { mobileOrder: 1 },
      },
      {
        accessorKey: 'project',
        header: t('Application project'),
        cell: ({ row }) => (
          <span className='font-medium'>{row.original.project}</span>
        ),
        size: 220,
        meta: { mobileTitle: true },
      },
      {
        accessorKey: 'requested_quota',
        header: t('Requested quota'),
        cell: ({ row }) => (
          <span className='tabular-nums'>
            {quotaToDisplayAmount(row.original.requested_quota)} {currencyLabel}
          </span>
        ),
        size: 160,
        meta: { mobileOrder: 2 },
      },
      {
        accessorKey: 'status',
        header: t('Application status'),
        cell: ({ row }) => {
          const status = TEMPORARY_QUOTA_REQUEST_STATUSES[row.original.status]
          return (
            <StatusBadge
              label={t(status.labelKey)}
              variant={status.variant}
              copyable={false}
            />
          )
        },
        size: 130,
        meta: { mobileBadge: true },
      },
      {
        accessorKey: 'approved_quota',
        header: t('Approved quota'),
        cell: ({ row }) =>
          row.original.status === 'executed' ? (
            <span className='tabular-nums'>
              {quotaToDisplayAmount(row.original.approved_quota)}{' '}
              {currencyLabel}
            </span>
          ) : (
            <span className='text-muted-foreground'>—</span>
          ),
        size: 160,
        meta: { mobileOrder: 3 },
      },
      {
        id: 'actions',
        header: t('Actions'),
        cell: ({ row }) => (
          <Button
            size='sm'
            variant='ghost'
            onClick={() => onView(row.original)}
          >
            <Eye className='size-4' aria-hidden='true' />
            {t('View')}
          </Button>
        ),
        enableSorting: false,
        enableHiding: false,
        size: 100,
      },
    ],
    [currencyLabel, onView, t]
  )

  const {
    globalFilter,
    onGlobalFilterChange,
    columnFilters,
    onColumnFiltersChange,
    pagination,
    onPaginationChange,
    ensurePageInRange,
  } = useTableUrlState({
    search: route.useSearch(),
    navigate: route.useNavigate(),
    pagination: { defaultPage: 1, defaultPageSize: 20 },
    globalFilter: { enabled: true, key: 'filter' },
    columnFilters: [{ columnId: 'status', searchKey: 'status', type: 'array' }],
  })
  const statusFilter =
    (columnFilters.find((filter) => filter.id === 'status')?.value as
      | string[]
      | undefined) ?? []
  const status = statusFilter[0] ?? ''
  const query = useQuery({
    queryKey: [
      ...temporaryQuotaQueryKeys.requests,
      pagination.pageIndex + 1,
      pagination.pageSize,
      status,
      globalFilter,
    ],
    queryFn: () =>
      listSelfTemporaryQuotaRequests({
        page: pagination.pageIndex + 1,
        pageSize: pagination.pageSize,
        status: status || undefined,
        keyword: globalFilter || undefined,
      }),
    placeholderData: (previousData) => previousData,
  })
  const { table } = useDataTable({
    data: query.data?.items ?? [],
    columns,
    columnFilters,
    globalFilter,
    pagination,
    onPaginationChange,
    onGlobalFilterChange,
    onColumnFiltersChange,
    manualPagination: true,
    manualFiltering: true,
    totalCount: query.data?.total ?? 0,
    ensurePageInRange,
  })
  const statusOptions = useMemo(
    () =>
      TEMPORARY_QUOTA_REQUEST_STATUS_OPTIONS.map((option) => ({
        ...option,
        label: t(option.label),
      })),
    [t]
  )

  if (query.isError) {
    return (
      <Alert variant='destructive'>
        <AlertTitle>{t('Failed to load temporary quota requests')}</AlertTitle>
        <AlertDescription>
          {getServerErrorMessage(query.error)}
        </AlertDescription>
        <AlertAction>
          <Button size='sm' variant='outline' onClick={() => query.refetch()}>
            {t('Retry')}
          </Button>
        </AlertAction>
      </Alert>
    )
  }

  return (
    <DataTablePage
      table={table}
      columns={columns}
      isLoading={query.isPending}
      isFetching={query.isFetching}
      emptyTitle={t('No temporary quota requests')}
      emptyDescription={t(
        'Submitted temporary quota requests will appear here'
      )}
      skeletonKeyPrefix='temporary-quota-skeleton'
      applyHeaderSize
      toolbarProps={{
        searchPlaceholder: t('Search by project or reason'),
        searchDebounceMs: 500,
        filters: [
          {
            columnId: 'status',
            title: t('Application status'),
            options: statusOptions,
            singleSelect: true,
          },
        ],
      }}
      getColumnClassName={(columnId) =>
        columnId === 'requested_quota' ||
        columnId === 'approved_quota' ||
        columnId === 'actions'
          ? 'text-right'
          : undefined
      }
    />
  )
}
