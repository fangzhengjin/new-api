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

import { deletePlan, regeneratePlan } from '../api'
import type { QuotaPlan } from '../types'
import { formatDateTime, formatQuota, queryKeys } from '../utils'
import { EmptyData, PlanTypeBadge, StatusBadge } from './shared'

type ListAction = 'regenerate' | 'delete'

function PlanListActions(props: { plan: QuotaPlan }) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const [action, setAction] = useState<ListAction | null>(null)
  const mutation = useMutation({
    mutationFn: async (selected: ListAction) => {
      if (selected === 'delete') {
        await deletePlan(props.plan.id)
        return { action: selected } as const
      }
      const result = await regeneratePlan(props.plan.id)
      return { action: selected, planId: result.plan_id } as const
    },
    onSuccess: async (result) => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: queryKeys.plans }),
        queryClient.invalidateQueries({
          queryKey: queryKeys.plan(props.plan.id),
        }),
        queryClient.invalidateQueries({
          queryKey: queryKeys.cycle(props.plan.cycle_id),
        }),
      ])
      setAction(null)
      if (result.action === 'delete') {
        toast.success(t('Quota plan deleted'))
        return
      }
      toast.success(t('Quota plan regenerated'))
      await navigate({
        to: '/quota-management/plans/$planId',
        params: { planId: String(result.planId) },
      })
    },
    onError: handleServerError,
  })

  if (props.plan.status === 'executed') return null

  return (
    <>
      <Button
        size='sm'
        variant='outline'
        disabled={mutation.isPending}
        onClick={() => setAction('regenerate')}
      >
        {t('Regenerate')}
      </Button>
      <Button
        size='sm'
        variant='destructive'
        disabled={mutation.isPending}
        onClick={() => setAction('delete')}
      >
        {t('Delete')}
      </Button>
      <AlertDialog
        open={action !== null}
        onOpenChange={(open) => !open && setAction(null)}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              {action === 'delete'
                ? t('Delete this quota plan?')
                : t('Regenerate this quota plan?')}
            </AlertDialogTitle>
            <AlertDialogDescription>
              {action === 'delete'
                ? t(
                    'The draft and every adjustment item will be permanently removed.'
                  )
                : t(
                    'A new draft uses the saved parameters and a fresh snapshot. The current draft is cancelled.'
                  )}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={mutation.isPending}>
              {t('Cancel')}
            </AlertDialogCancel>
            <AlertDialogAction
              variant={action === 'delete' ? 'destructive' : 'default'}
              disabled={!action || mutation.isPending}
              onClick={() => action && mutation.mutate(action)}
            >
              {mutation.isPending && <Spinner data-icon='inline-start' />}
              {action === 'delete' ? t('Delete') : t('Regenerate')}
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
        title={t('No quota plans')}
        description={t('Generate a draft after an active cycle is available.')}
      />
    )
  }
  return (
    <div className='rounded-xl border'>
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>{t('Plan')}</TableHead>
            <TableHead>{t('Type')}</TableHead>
            <TableHead>{t('Stage')}</TableHead>
            <TableHead>{t('Planned change')}</TableHead>
            <TableHead>{t('Status')}</TableHead>
            <TableHead>{t('Snapshot')}</TableHead>
            <TableHead>{t('Created at')}</TableHead>
            <TableHead className='text-right'>{t('Actions')}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {props.plans.map((plan) => (
            <TableRow key={plan.id}>
              <TableCell className='font-medium'>#{plan.id}</TableCell>
              <TableCell>
                <PlanTypeBadge
                  type={plan.plan_type}
                  thorough={plan.parameters.thorough_release === true}
                  t={t}
                />
              </TableCell>
              <TableCell>{plan.stage_percent / 100}%</TableCell>
              <TableCell>
                {formatQuota(plan.planned_delta_quota, true)}
              </TableCell>
              <TableCell>
                <StatusBadge value={plan.status} t={t} />
              </TableCell>
              <TableCell>{formatDateTime(plan.snapshot_at)}</TableCell>
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
