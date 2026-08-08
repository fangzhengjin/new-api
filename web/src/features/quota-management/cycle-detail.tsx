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
import { zodResolver } from '@hookform/resolvers/zod'
import {
  ArrowLeft01Icon,
  Coins01Icon,
  Edit02Icon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, useNavigate } from '@tanstack/react-router'
import { useEffect, useMemo, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { z } from 'zod'

import { SectionPageLayout } from '@/components/layout'
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
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  Field,
  FieldError,
  FieldGroup,
  FieldLabel,
} from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Spinner } from '@/components/ui/spinner'
import { getCurrencyDisplay, getCurrencyLabel } from '@/lib/currency'
import { handleServerError } from '@/lib/handle-server-error'

import { closeCycle, getCycle, getPlanOptions, updateCycle } from './api'
import { GeneratePlanDialog } from './components/generate-plan-dialog'
import { PlanTable } from './components/plan-table'
import {
  InitialGrantRecommendationAlert,
  PageError,
  PageLoading,
  StatusBadge,
} from './components/shared'
import {
  formatDateTime,
  formatQuota,
  queryKeys,
  quotaFromDisplayAmount,
  quotaToDisplayAmount,
} from './utils'

type FormValues = { budgetAmount: string; initialGrantAmount: string }
const currencyAmountPattern = /^\d+(?:\.\d{1,6})?$/
const tokenAmountPattern = /^\d+$/

export function QuotaCycleDetail(props: { cycleId: number }) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const { meta: currencyMeta } = getCurrencyDisplay()
  const currencyLabel = getCurrencyLabel()
  const tokensOnly = currencyMeta.kind === 'tokens'
  const amountPattern = tokensOnly ? tokenAmountPattern : currencyAmountPattern
  const amountError = tokensOnly
    ? t('Enter quota in tokens')
    : t('Enter quota in {{currency}}', { currency: currencyLabel })
  const [editOpen, setEditOpen] = useState(false)
  const [closeOpen, setCloseOpen] = useState(false)
  const [planOpen, setPlanOpen] = useState(false)
  const query = useQuery({
    queryKey: queryKeys.cycle(props.cycleId),
    queryFn: () => getCycle(props.cycleId),
  })
  const optionsQuery = useQuery({
    queryKey: queryKeys.options,
    queryFn: getPlanOptions,
  })
  const schema = useMemo(
    () =>
      z.object({
        budgetAmount: z
          .string()
          .regex(amountPattern, amountError)
          .refine(
            (value) => quotaFromDisplayAmount(value) !== null,
            amountError
          ),
        initialGrantAmount: z
          .string()
          .regex(amountPattern, amountError)
          .refine(
            (value) => quotaFromDisplayAmount(value) !== null,
            amountError
          ),
      }),
    [amountError, amountPattern]
  )
  const form = useForm<FormValues>({
    resolver: zodResolver(schema),
    defaultValues: { budgetAmount: '', initialGrantAmount: '' },
  })

  useEffect(() => {
    if (!editOpen || !query.data) return
    form.reset({
      budgetAmount: quotaToDisplayAmount(query.data.cycle.budget_quota),
      initialGrantAmount: quotaToDisplayAmount(
        query.data.cycle.initial_grant_quota
      ),
    })
  }, [editOpen, form, query.data])

  const editMutation = useMutation({
    mutationFn: (values: FormValues) => {
      const budgetQuota = quotaFromDisplayAmount(values.budgetAmount)
      const initialGrantQuota = quotaFromDisplayAmount(
        values.initialGrantAmount
      )
      if (!budgetQuota || !initialGrantQuota) {
        throw new Error(amountError)
      }
      return updateCycle(props.cycleId, {
        budget_quota: budgetQuota,
        initial_grant_quota:
          query.data?.cycle.status === 'scheduled'
            ? initialGrantQuota
            : undefined,
      })
    },
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({
          queryKey: queryKeys.cycle(props.cycleId),
        }),
        queryClient.invalidateQueries({ queryKey: queryKeys.cycles }),
        queryClient.invalidateQueries({ queryKey: queryKeys.options }),
      ])
      toast.success(t('Quota cycle updated'))
      setEditOpen(false)
    },
    onError: handleServerError,
  })
  const closeMutation = useMutation({
    mutationFn: () => closeCycle(props.cycleId),
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: queryKeys.cycles }),
        queryClient.invalidateQueries({ queryKey: queryKeys.plans }),
        queryClient.invalidateQueries({ queryKey: queryKeys.options }),
      ])
      toast.success(t('Quota cycle closed'))
      await navigate({ to: '/quota-management' })
    },
    onError: handleServerError,
  })

  if (query.isPending) {
    return (
      <SectionPageLayout>
        <SectionPageLayout.Title>{t('Quota cycle')}</SectionPageLayout.Title>
        <SectionPageLayout.Content>
          <PageLoading />
        </SectionPageLayout.Content>
      </SectionPageLayout>
    )
  }
  if (query.isError) {
    return (
      <SectionPageLayout>
        <SectionPageLayout.Title>{t('Quota cycle')}</SectionPageLayout.Title>
        <SectionPageLayout.Content>
          <PageError
            title={t('Failed to load quota cycle')}
            error={query.error}
          />
        </SectionPageLayout.Content>
      </SectionPageLayout>
    )
  }
  const { cycle, plans, recommendation } = query.data

  return (
    <>
      <SectionPageLayout>
        <SectionPageLayout.Title>
          {t('Quota cycle')} #{cycle.id}
        </SectionPageLayout.Title>
        <SectionPageLayout.Breadcrumb>
          <Button
            variant='ghost'
            size='sm'
            className='text-muted-foreground h-auto px-0 py-1 text-xs'
            render={<Link to='/quota-management' />}
          >
            <HugeiconsIcon icon={ArrowLeft01Icon} strokeWidth={2} />
            {t('Back to quota management')}
          </Button>
        </SectionPageLayout.Breadcrumb>
        <SectionPageLayout.Actions>
          <Button
            variant='outline'
            onClick={() => setPlanOpen(true)}
            disabled={
              cycle.status === 'closed' ||
              optionsQuery.data?.cycle?.id !== cycle.id
            }
          >
            <HugeiconsIcon
              icon={Coins01Icon}
              strokeWidth={2}
              data-icon='inline-start'
            />
            {t('Generate plan')}
          </Button>
          <Button
            variant='outline'
            onClick={() => setEditOpen(true)}
            disabled={cycle.status === 'closed'}
          >
            <HugeiconsIcon
              icon={Edit02Icon}
              strokeWidth={2}
              data-icon='inline-start'
            />
            {t('Edit cycle')}
          </Button>
          <Button
            variant='destructive'
            onClick={() => setCloseOpen(true)}
            disabled={cycle.status === 'closed'}
          >
            {t('Close cycle')}
          </Button>
        </SectionPageLayout.Actions>
        <SectionPageLayout.Content>
          <div className='grid gap-4'>
            {optionsQuery.isError && (
              <PageError
                title={t('Generate plan')}
                error={optionsQuery.error}
              />
            )}
            {recommendation && (
              <InitialGrantRecommendationAlert
                recommendation={recommendation}
                t={t}
              />
            )}
            <div className='grid gap-4 sm:grid-cols-2 lg:grid-cols-4'>
              <Card>
                <CardHeader>
                  <CardDescription>{t('Status')}</CardDescription>
                  <CardTitle>
                    <StatusBadge value={cycle.status} t={t} />
                  </CardTitle>
                </CardHeader>
                <CardContent>
                  {formatDateTime(cycle.cycle_start_at)} –{' '}
                  {formatDateTime(cycle.cycle_end_at)}
                </CardContent>
              </Card>
              <Card>
                <CardHeader>
                  <CardDescription>{t('Budget')}</CardDescription>
                  <CardTitle>{formatQuota(cycle.budget_quota)}</CardTitle>
                </CardHeader>
                <CardContent>{t('Purchasing pool')}</CardContent>
              </Card>
              <Card>
                <CardHeader>
                  <CardDescription>
                    {t('Initial grant per user')}
                  </CardDescription>
                  <CardTitle>
                    {formatQuota(cycle.initial_grant_quota)}
                  </CardTitle>
                </CardHeader>
                <CardContent>{t('Applied by initialization')}</CardContent>
              </Card>
              <Card>
                <CardHeader>
                  <CardDescription>{t('Last updated')}</CardDescription>
                  <CardTitle>{formatDateTime(cycle.updated_at)}</CardTitle>
                </CardHeader>
                <CardContent>
                  {cycle.updated_by || cycle.created_by || '—'}
                </CardContent>
              </Card>
            </div>
            <Card>
              <CardHeader>
                <CardTitle>{t('Related quota plans')}</CardTitle>
                <CardDescription>
                  {t('Drafts and execution history for this purchasing cycle.')}
                </CardDescription>
              </CardHeader>
              <CardContent>
                <PlanTable plans={plans} />
              </CardContent>
            </Card>
          </div>
        </SectionPageLayout.Content>
      </SectionPageLayout>

      <GeneratePlanDialog
        open={planOpen}
        onOpenChange={setPlanOpen}
        options={optionsQuery.data}
      />

      <Dialog open={editOpen} onOpenChange={setEditOpen}>
        <DialogContent className='sm:max-w-md'>
          <DialogHeader>
            <DialogTitle>{t('Edit quota cycle')}</DialogTitle>
            <DialogDescription>
              {t('Active cycles allow budget changes only.')}
            </DialogDescription>
          </DialogHeader>
          <form
            id='edit-quota-cycle'
            onSubmit={form.handleSubmit((values) =>
              editMutation.mutate(values)
            )}
          >
            <FieldGroup>
              <Field data-invalid={!!form.formState.errors.budgetAmount}>
                <FieldLabel htmlFor='edit-cycle-budget'>
                  {t('Budget')} ({currencyLabel})
                </FieldLabel>
                <Input
                  id='edit-cycle-budget'
                  inputMode='decimal'
                  {...form.register('budgetAmount')}
                />
                <FieldError errors={[form.formState.errors.budgetAmount]} />
              </Field>
              <Field
                data-disabled={cycle.status !== 'scheduled'}
                data-invalid={!!form.formState.errors.initialGrantAmount}
              >
                <FieldLabel htmlFor='edit-cycle-grant'>
                  {t('Initial grant per user')} ({currencyLabel})
                </FieldLabel>
                <Input
                  id='edit-cycle-grant'
                  inputMode='decimal'
                  disabled={cycle.status !== 'scheduled'}
                  {...form.register('initialGrantAmount')}
                />
                <FieldError
                  errors={[form.formState.errors.initialGrantAmount]}
                />
              </Field>
            </FieldGroup>
          </form>
          <DialogFooter>
            <Button
              type='button'
              variant='outline'
              disabled={editMutation.isPending}
              onClick={() => setEditOpen(false)}
            >
              {t('Cancel')}
            </Button>
            <Button
              type='submit'
              form='edit-quota-cycle'
              disabled={editMutation.isPending}
            >
              {editMutation.isPending && <Spinner data-icon='inline-start' />}
              {t('Save')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <AlertDialog open={closeOpen} onOpenChange={setCloseOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t('Close this quota cycle?')}</AlertDialogTitle>
            <AlertDialogDescription>
              {t(
                'This closes the cycle and cancels every remaining draft. This action cannot be undone.'
              )}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={closeMutation.isPending}>
              {t('Cancel')}
            </AlertDialogCancel>
            <AlertDialogAction
              variant='destructive'
              disabled={closeMutation.isPending}
              onClick={() => closeMutation.mutate()}
            >
              {closeMutation.isPending && <Spinner data-icon='inline-start' />}
              {t('Close cycle')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  )
}
