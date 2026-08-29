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
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { useEffect, useMemo } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { z } from 'zod'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
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
  FieldDescription,
  FieldError,
  FieldGroup,
  FieldLabel,
} from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Spinner } from '@/components/ui/spinner'
import { Textarea } from '@/components/ui/textarea'
import { handleServerError } from '@/lib/handle-server-error'

import { generatePlan } from '../api'
import type { PlanOptions, PlanWrite, QuotaCycle } from '../types'
import {
  formatDateTime,
  formatQuota,
  fromShanghaiAdjustmentDate,
  queryKeys,
  toShanghaiDateInput,
} from '../utils'

type FormValues = {
  name: string
  purpose: string
  nextAdjustmentAt: string
}

function planWriteFromForm(
  values: FormValues,
  cycle: QuotaCycle,
  initializationRequired: boolean
): PlanWrite {
  return {
    cycle_id: cycle.id,
    name: values.name.trim(),
    purpose: values.purpose.trim(),
    plan_type: initializationRequired ? 'initialization' : 'adjustment',
    next_adjustment_at: fromShanghaiAdjustmentDate(
      values.nextAdjustmentAt,
      cycle.cycle_end_at
    ),
  }
}

export function GeneratePlanDialog(props: {
  open: boolean
  onOpenChange: (open: boolean) => void
  options?: PlanOptions
}) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const schema = useMemo(
    () =>
      z.object({
        name: z
          .string()
          .trim()
          .min(1, t('Plan name is required'))
          .max(64, t('Plan name must be 64 characters or fewer')),
        purpose: z
          .string()
          .trim()
          .min(1, t('Plan purpose is required'))
          .max(200, t('Plan purpose must be 200 characters or fewer')),
        nextAdjustmentAt: z
          .string()
          .min(1, t('Next adjustment time is required')),
      }),
    [t]
  )
  const form = useForm<FormValues>({
    resolver: zodResolver(schema),
    defaultValues: {
      name: '',
      purpose: '',
      nextAdjustmentAt: '',
    },
  })

  useEffect(() => {
    if (!props.open || !props.options?.defaults) return
    form.reset({
      name: props.options.initialization_required
        ? t('Initial allocation')
        : '',
      purpose: props.options.initialization_required
        ? t('Set the starting quota for managed users')
        : '',
      nextAdjustmentAt: toShanghaiDateInput(
        props.options.defaults.next_adjustment_at
      ),
    })
  }, [form, props.open, props.options, t])

  const mutation = useMutation({
    mutationFn: generatePlan,
    onSuccess: async ({ plan_id }) => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: queryKeys.overview }),
        queryClient.invalidateQueries({ queryKey: queryKeys.activities }),
        queryClient.invalidateQueries({ queryKey: queryKeys.options }),
        queryClient.invalidateQueries({
          queryKey: queryKeys.cycle(props.options?.cycle?.id ?? 0),
        }),
      ])
      toast.success(t('Quota plan generated'))
      props.onOpenChange(false)
      await navigate({
        to: '/quota-management/plans/$planId',
        params: { planId: String(plan_id) },
      })
    },
    onError: handleServerError,
  })

  const submit = form.handleSubmit((values) => {
    if (!props.options?.cycle) return
    const nextAdjustmentAt = fromShanghaiAdjustmentDate(
      values.nextAdjustmentAt,
      props.options.cycle.cycle_end_at
    )
    if (
      nextAdjustmentAt <= Math.floor(Date.now() / 1000) ||
      nextAdjustmentAt > props.options.cycle.cycle_end_at
    ) {
      form.setError('nextAdjustmentAt', {
        message: t(
          'Next adjustment must be later than now and within the cycle'
        ),
      })
      return
    }
    mutation.mutate(
      planWriteFromForm(
        values,
        props.options.cycle,
        props.options.initialization_required
      )
    )
  })

  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      <DialogContent className='max-h-[90vh] overflow-y-auto sm:max-w-xl'>
        <DialogHeader>
          <DialogTitle>{t('Create allocation plan')}</DialogTitle>
          <DialogDescription>
            {t(
              'Name this allocation plan and choose when the next review should happen, the draft will show every planned issue and reclaim before execution'
            )}
          </DialogDescription>
        </DialogHeader>
        {!props.options?.cycle && (
          <Alert variant='destructive'>
            <AlertTitle>{t('No active cycle')}</AlertTitle>
            <AlertDescription>
              {t(
                'Wait for a scheduled cycle to start before generating a plan'
              )}
            </AlertDescription>
          </Alert>
        )}
        {props.options?.cycle && (
          <form id='generate-quota-plan' onSubmit={submit}>
            <FieldGroup>
              <Alert>
                <AlertTitle>
                  {t('Current cycle')} #{props.options.cycle.id}
                </AlertTitle>
                <AlertDescription>
                  {formatDateTime(props.options.cycle.cycle_start_at)} –{' '}
                  {formatDateTime(props.options.cycle.cycle_end_at)} ·{' '}
                  {t('Cycle total')}:{' '}
                  {formatQuota(props.options.cycle.budget_quota)}
                </AlertDescription>
              </Alert>
              {props.options.initialization_required && (
                <Alert>
                  <AlertTitle>{t('First allocation')}</AlertTitle>
                  <AlertDescription>
                    {t(
                      'This will be the first plan in the cycle. The system will calculate how much each managed user needs to reach the starting amount'
                    )}
                  </AlertDescription>
                </Alert>
              )}
              <Field data-invalid={!!form.formState.errors.name}>
                <FieldLabel htmlFor='quota-plan-name'>
                  {t('Plan name')}
                </FieldLabel>
                <Input
                  id='quota-plan-name'
                  aria-invalid={!!form.formState.errors.name}
                  aria-describedby={
                    form.formState.errors.name
                      ? 'quota-plan-name-description quota-plan-name-error'
                      : 'quota-plan-name-description'
                  }
                  maxLength={64}
                  placeholder={t('Example: New project allocation')}
                  {...form.register('name')}
                />
                <FieldDescription id='quota-plan-name-description'>
                  {t('Use a short name that identifies this allocation plan')}
                </FieldDescription>
                <FieldError
                  id='quota-plan-name-error'
                  errors={[form.formState.errors.name]}
                />
              </Field>
              <Field data-invalid={!!form.formState.errors.purpose}>
                <FieldLabel htmlFor='quota-plan-purpose'>
                  {t('Purpose')}
                </FieldLabel>
                <Textarea
                  id='quota-plan-purpose'
                  aria-invalid={!!form.formState.errors.purpose}
                  aria-describedby={
                    form.formState.errors.purpose
                      ? 'quota-plan-purpose-error'
                      : undefined
                  }
                  maxLength={200}
                  rows={3}
                  placeholder={t('Explain why this allocation is needed')}
                  {...form.register('purpose')}
                />
                <FieldError
                  id='quota-plan-purpose-error'
                  errors={[form.formState.errors.purpose]}
                />
              </Field>
              <Field data-invalid={!!form.formState.errors.nextAdjustmentAt}>
                <FieldLabel htmlFor='quota-next-adjustment'>
                  {t('Next planned adjustment')}
                </FieldLabel>
                <Input
                  id='quota-next-adjustment'
                  aria-invalid={!!form.formState.errors.nextAdjustmentAt}
                  aria-describedby={
                    form.formState.errors.nextAdjustmentAt
                      ? 'quota-next-adjustment-description quota-next-adjustment-error'
                      : 'quota-next-adjustment-description'
                  }
                  type='date'
                  min={toShanghaiDateInput(Math.floor(Date.now() / 1000))}
                  max={toShanghaiDateInput(props.options.cycle.cycle_end_at)}
                  {...form.register('nextAdjustmentAt')}
                />
                <FieldDescription id='quota-next-adjustment-description'>
                  {t(
                    'This is a reminder for the next review and does not define the plan duration'
                  )}
                </FieldDescription>
                <FieldError
                  id='quota-next-adjustment-error'
                  errors={[form.formState.errors.nextAdjustmentAt]}
                />
              </Field>
            </FieldGroup>
          </form>
        )}
        <DialogFooter>
          <Button
            type='button'
            variant='outline'
            disabled={mutation.isPending}
            onClick={() => props.onOpenChange(false)}
          >
            {t('Cancel')}
          </Button>
          <Button
            type='submit'
            form='generate-quota-plan'
            disabled={!props.options?.cycle || mutation.isPending}
          >
            {mutation.isPending && <Spinner data-icon='inline-start' />}
            {t('Create draft')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
