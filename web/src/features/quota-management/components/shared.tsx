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
import { File02Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import type { TFunction } from 'i18next'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from '@/components/ui/empty'
import { Skeleton } from '@/components/ui/skeleton'
import { getServerErrorMessage } from '@/lib/server-error-message'

import type {
  AdjustmentAction,
  CycleStatus,
  InitialGrantRecommendation,
  NotificationStatus,
  PlanStatus,
  PlanType,
} from '../types'
import { formatDateTime, formatQuota } from '../utils'

const labels = {
  scheduled: '已规划',
  active: 'Active',
  settling: '结算中',
  closed: '已关闭',
  draft: '草稿',
  executed: '已执行',
  cancelled: 'Cancelled',
  initialization: '首次发放',
  adjustment: '周期内调配',
  settlement: '周期收口',
  initialize: 'Initialize',
  increase: '调增',
  decrease: '调减',
  grant: '补发',
  reclaim: '回收',
  temporary_grant: '临时额度',
  pending: 'Pending',
  sent: '已发送',
  failed: 'Failed',
  skipped: '已跳过',
} as const

type StatusValue =
  | CycleStatus
  | PlanStatus
  | PlanType
  | AdjustmentAction
  | NotificationStatus

export function StatusBadge(props: { value: StatusValue; t: TFunction }) {
  const destructive = props.value === 'failed' || props.value === 'cancelled'
  const secondary =
    props.value === 'closed' ||
    props.value === 'skipped' ||
    props.value === 'scheduled'
  let variant: 'destructive' | 'secondary' | 'outline' = 'outline'
  if (destructive) variant = 'destructive'
  else if (secondary) variant = 'secondary'
  return <Badge variant={variant}>{props.t(labels[props.value])}</Badge>
}

export function PlanTypeBadge(props: { type: PlanType; t: TFunction }) {
  return <Badge variant='outline'>{props.t(labels[props.type])}</Badge>
}

export function InitialGrantRecommendationAlert(props: {
  recommendation: InitialGrantRecommendation
  t: TFunction
}) {
  return (
    <Alert>
      <AlertTitle>{props.t('建议的每人首次发放额度')}</AlertTitle>
      <AlertDescription>
        {formatQuota(props.recommendation.quota)} ·{' '}
        {props.recommendation.user_count} {props.t('名用户')} ·{' '}
        {formatDateTime(props.recommendation.reference_start)} –{' '}
        {formatDateTime(props.recommendation.reference_end)}
        <br />
        {props.t('人均实际使用额度')}:{' '}
        {formatQuota(props.recommendation.average_spend)} ·{' '}
        {props.t('在人均实际使用额度上多留 20%')}:{' '}
        {formatQuota(props.recommendation.buffered)} ·{' '}
        {props.t('至少保留上期首次额度的 80%')}:{' '}
        {formatQuota(props.recommendation.stability_floor)}
      </AlertDescription>
    </Alert>
  )
}

export function PageLoading() {
  return (
    <div className='grid gap-3'>
      <Skeleton className='h-24 w-full' />
      <Skeleton className='h-64 w-full' />
    </div>
  )
}

export function PageError(props: { title: string; error: unknown }) {
  return (
    <Alert variant='destructive'>
      <AlertTitle>{props.title}</AlertTitle>
      <AlertDescription>{getServerErrorMessage(props.error)}</AlertDescription>
    </Alert>
  )
}

export function EmptyData(props: { title: string; description: string }) {
  return (
    <Empty className='min-h-56 border'>
      <EmptyHeader>
        <EmptyMedia variant='icon'>
          <HugeiconsIcon icon={File02Icon} strokeWidth={2} />
        </EmptyMedia>
        <EmptyTitle>{props.title}</EmptyTitle>
        <EmptyDescription>{props.description}</EmptyDescription>
      </EmptyHeader>
    </Empty>
  )
}
