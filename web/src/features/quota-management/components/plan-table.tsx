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
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Link, useNavigate } from '@tanstack/react-router'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { Button } from '@/components/ui/button'
import { Spinner } from '@/components/ui/spinner'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { handleServerError } from '@/lib/handle-server-error'

import { regeneratePlan } from '../api'
import type { QuotaPlan } from '../types'
import { formatDateTime, formatQuota, queryKeys } from '../utils'
import { EmptyData, PlanTypeBadge, StatusBadge } from './shared'

function PlanListActions(props: { plan: QuotaPlan }) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const [open, setOpen] = useState(false)
  const mutation = useMutation({
    mutationFn: () => regeneratePlan(props.plan.id),
    onSuccess: async (result) => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: queryKeys.overview }),
        queryClient.invalidateQueries({
          queryKey: queryKeys.plan(props.plan.id),
        }),
        queryClient.invalidateQueries({
          queryKey: queryKeys.cycle(props.plan.cycle_id),
        }),
      ])
      setOpen(false)
      toast.success(t('方案已重新计算'))
      await navigate({
        to: '/quota-management/plans/$planId',
        params: { planId: String(result.plan_id) },
      })
    },
    onError: (error: unknown) => handleServerError(error),
  })

  if (props.plan.status === 'executed') return null

  return (
    <>
      <Button
        size='sm'
        variant='outline'
        disabled={mutation.isPending}
        onClick={() => setOpen(true)}
      >
        {t('重新计算')}
      </Button>
      <AlertDialog open={open} onOpenChange={setOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t('是否重新计算此方案')}</AlertDialogTitle>
            <AlertDialogDescription>
              {t('当前草稿将被取消，并将根据当前用户余额重新计算新草稿')}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={mutation.isPending}>
              {t('Cancel')}
            </AlertDialogCancel>
            <AlertDialogAction
              disabled={mutation.isPending}
              onClick={() => mutation.mutate(undefined)}
            >
              {mutation.isPending && <Spinner data-icon='inline-start' />}
              {t('重新计算')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  )
}

export function PlanTable(props: { plans: QuotaPlan[] }) {
  const { t } = useTranslation()
  if (!props.plans.length) {
    return (
      <EmptyData
        title={t('没有额度方案')}
        description={t('额度周期开始后，就可以在这里生成调配草稿')}
      />
    )
  }
  return (
    <div className='overflow-x-auto rounded-xl border'>
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>{t('Plan')}</TableHead>
            <TableHead>{t('分配类型')}</TableHead>
            <TableHead>{t('方案净变动')}</TableHead>
            <TableHead>{t('Status')}</TableHead>
            <TableHead>{t('创建时间')}</TableHead>
            <TableHead className='text-right'>{t('Actions')}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {props.plans.map((plan) => (
            <TableRow key={plan.id}>
              <TableCell>
                <div className='font-medium'>{plan.name}</div>
                {plan.purpose && (
                  <div className='text-muted-foreground mt-0.5 max-w-72 truncate text-xs'>
                    {plan.purpose}
                  </div>
                )}
              </TableCell>
              <TableCell>
                <PlanTypeBadge type={plan.plan_type} t={t} />
              </TableCell>
              <TableCell>
                {formatQuota(plan.planned_delta_quota, true)}
              </TableCell>
              <TableCell>
                <StatusBadge value={plan.status} t={t} />
              </TableCell>
              <TableCell>{formatDateTime(plan.created_at)}</TableCell>
              <TableCell className='text-right'>
                <div className='flex justify-end gap-2'>
                  <Button
                    size='sm'
                    variant='outline'
                    render={
                      <Link
                        to='/quota-management/plans/$planId'
                        params={{ planId: String(plan.id) }}
                      />
                    }
                  >
                    {t('View details')}
                  </Button>
                  <PlanListActions plan={plan} />
                </div>
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </div>
  )
}
