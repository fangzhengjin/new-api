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

import {
  sideDrawerContentClassName,
  sideDrawerFormClassName,
  sideDrawerHeaderClassName,
} from '@/components/drawer-layout'
import { StatusBadge } from '@/components/status-badge'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { getCurrencyLabel } from '@/lib/currency'

import {
  formatDateTime,
  quotaToDisplayAmount,
} from '../../quota-management/utils'
import { TEMPORARY_QUOTA_REQUEST_STATUSES } from '../constants'
import type { SelfTemporaryQuotaRequest } from '../types'

function DetailItem(props: { label: string; children: React.ReactNode }) {
  return (
    <div className='grid gap-1 border-b py-3 last:border-b-0 sm:grid-cols-[9rem_minmax(0,1fr)] sm:gap-4'>
      <dt className='text-muted-foreground text-sm'>{props.label}</dt>
      <dd className='min-w-0 text-sm break-words'>{props.children}</dd>
    </div>
  )
}

export function TemporaryQuotaDetailSheet(props: {
  request: SelfTemporaryQuotaRequest | null
  onOpenChange: (open: boolean) => void
}) {
  const { t } = useTranslation()
  const currencyLabel = getCurrencyLabel()
  const request = props.request
  const statusConfig = request
    ? TEMPORARY_QUOTA_REQUEST_STATUSES[request.status]
    : null
  const approvedQuota =
    request?.status === 'executed'
      ? `${quotaToDisplayAmount(request.approved_quota)} ${currencyLabel}`
      : '—'

  return (
    <Sheet
      open={request !== null}
      onOpenChange={(open) => props.onOpenChange(open)}
    >
      <SheetContent
        className={sideDrawerContentClassName('sm:max-w-lg')}
        aria-describedby='temporary-quota-detail-description'
      >
        <SheetHeader className={sideDrawerHeaderClassName()}>
          <SheetTitle>{t('Temporary quota request details')}</SheetTitle>
          <SheetDescription id='temporary-quota-detail-description'>
            {t('View the application information and review result')}
          </SheetDescription>
        </SheetHeader>
        {request && statusConfig && (
          <div className={sideDrawerFormClassName()}>
            <dl className='rounded-xl border px-4'>
              <DetailItem label={t('Application time')}>
                {formatDateTime(request.created_at)}
              </DetailItem>
              <DetailItem label={t('Application project')}>
                {request.project}
              </DetailItem>
              <DetailItem label={t('Requested quota')}>
                <span className='tabular-nums'>
                  {quotaToDisplayAmount(request.requested_quota)}{' '}
                  {currencyLabel}
                </span>
              </DetailItem>
              <DetailItem label={t('Application status')}>
                <StatusBadge
                  label={t(statusConfig.labelKey)}
                  variant={statusConfig.variant}
                  copyable={false}
                />
              </DetailItem>
              <DetailItem label={t('Approved quota')}>
                <span className='tabular-nums'>{approvedQuota}</span>
              </DetailItem>
              <DetailItem label={t('Reason')}>{request.reason}</DetailItem>
              {request.review_reason && (
                <DetailItem label={t('Review reason')}>
                  {request.review_reason}
                </DetailItem>
              )}
              {request.reviewed_at && (
                <DetailItem label={t('Reviewed at')}>
                  {formatDateTime(request.reviewed_at)}
                </DetailItem>
              )}
              {request.executed_at && (
                <DetailItem label={t('Granted at')}>
                  {formatDateTime(request.executed_at)}
                </DetailItem>
              )}
            </dl>
          </div>
        )}
      </SheetContent>
    </Sheet>
  )
}
