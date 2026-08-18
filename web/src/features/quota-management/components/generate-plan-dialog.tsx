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
import { useEffect, useMemo, useState } from 'react'
import { Controller, useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { z } from 'zod'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
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
  FieldContent,
  FieldError,
  FieldGroup,
  FieldLabel,
} from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Spinner } from '@/components/ui/spinner'
import { handleServerError } from '@/lib/handle-server-error'

import {
  comparePlanFairness,
  generatePlan,
  recordFairnessEvidence,
} from '../api'
import type {
  FairnessShadowComparison,
  GeneratablePlanType,
  PlanOptions,
  PlanWrite,
  QuotaCycle,
} from '../types'
import {
  formatConcentrationMultiplier,
  formatDateTime,
  formatQuota,
  fromShanghaiAdjustmentDate,
  queryKeys,
  toShanghaiDateInput,
} from '../utils'
import { FairnessShadowResult } from './fairness-shadow-comparison'

type FormValues = {
  planType: GeneratablePlanType
  stagePercent: '0' | '7500' | '8500' | '9500' | '10000'
  nextAdjustmentAt: string
  basisMode: 'actual' | 'week'
  earlyReclaim: boolean
  reclaimCapPercent: string
  usageBonusPercent: string
  thorough: boolean
}

function planWriteFromForm(values: FormValues, cycle: QuotaCycle): PlanWrite {
  let usageBonusPercent =
    values.usageBonusPercent.trim() === ''
      ? 30
      : Number(values.usageBonusPercent)
  if (cycle.concentration_multiplier_basis_points > 0) {
    usageBonusPercent = 0
  }
  return {
    cycle_id: cycle.id,
    plan_type: values.planType,
    stage_percent: Number(values.stagePercent),
    next_adjustment_at: fromShanghaiAdjustmentDate(
      values.nextAdjustmentAt,
      cycle.cycle_end_at
    ),
    basis_mode: values.basisMode,
    early_reclaim: values.earlyReclaim,
    reclaim_cap_percent:
      values.reclaimCapPercent.trim() === ''
        ? 30
        : Number(values.reclaimCapPercent),
    usage_bonus_percent: usageBonusPercent,
    thorough_release: values.thorough,
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
  const [shadowResult, setShadowResult] =
    useState<FairnessShadowComparison | null>(null)
  const [shadowParams, setShadowParams] = useState<PlanWrite | null>(null)
  const concentrationMultiplier =
    props.options?.cycle?.concentration_multiplier_basis_points ?? 0
  const usesCycleLimit = concentrationMultiplier > 0
  const schema = useMemo(
    () =>
      z.object({
        planType: z.enum(['initialization', 'adjustment']),
        stagePercent: z.enum(['0', '7500', '8500', '9500', '10000']),
        nextAdjustmentAt: z
          .string()
          .min(1, t('Next adjustment time is required')),
        basisMode: z.enum(['actual', 'week']),
        earlyReclaim: z.boolean(),
        thorough: z.boolean(),
        reclaimCapPercent: z
          .string()
          .refine(
            (value) =>
              value.trim() === '' ||
              (Number.isInteger(Number(value)) &&
                Number(value) >= 0 &&
                Number(value) <= 100),
            t('Enter a percentage from 0 to 100')
          ),
        usageBonusPercent: z
          .string()
          .refine(
            (value) =>
              value.trim() === '' ||
              (Number.isInteger(Number(value)) &&
                Number(value) >= 0 &&
                Number(value) <= 100),
            t('Enter a percentage from 0 to 100')
          ),
      }),
    [t]
  )
  const form = useForm<FormValues>({
    resolver: zodResolver(schema),
    defaultValues: {
      planType: 'adjustment',
      stagePercent: '7500',
      nextAdjustmentAt: '',
      basisMode: 'actual',
      earlyReclaim: false,
      reclaimCapPercent: '30',
      usageBonusPercent: '30',
      thorough: false,
    },
  })

  useEffect(() => {
    if (!props.open || !props.options?.defaults) return
    const planType = props.options.defaults.plan_type
    form.reset({
      planType,
      stagePercent: String(
        props.options.defaults.stage_percent
      ) as FormValues['stagePercent'],
      nextAdjustmentAt: toShanghaiDateInput(
        props.options.defaults.next_adjustment_at
      ),
      basisMode: props.options.defaults.basis_mode,
      earlyReclaim: props.options.defaults.early_reclaim,
      reclaimCapPercent: String(props.options.defaults.reclaim_cap_percent),
      usageBonusPercent: String(props.options.defaults.usage_bonus_percent),
      thorough: false,
    })
  }, [form, props.open, props.options])

  useEffect(() => {
    const subscription = form.watch(() => {
      setShadowResult(null)
      setShadowParams(null)
    })
    return () => subscription.unsubscribe()
  }, [form])

  const mutation = useMutation({
    mutationFn: generatePlan,
    onSuccess: async ({ plan_id }) => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: queryKeys.plans }),
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
  const shadowMutation = useMutation({
    mutationFn: comparePlanFairness,
    onSuccess: (result, params) => {
      setShadowResult(result)
      setShadowParams(params)
    },
    onError: handleServerError,
  })
  const evidenceMutation = useMutation({
    mutationFn: recordFairnessEvidence,
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: queryKeys.algorithm })
      toast.success(t('Qualified shadow evidence recorded'))
    },
    onError: handleServerError,
  })

  const thorough = form.watch('thorough')
  const disabled =
    !props.options?.cycle || (thorough && props.options.initialization_required)
  const planType = form.watch('planType')
  const stagePercent = form.watch('stagePercent')

  useEffect(() => {
    if (thorough && props.options?.cycle) {
      form.setValue('planType', 'adjustment')
      form.setValue('stagePercent', '10000')
      form.setValue(
        'nextAdjustmentAt',
        toShanghaiDateInput(props.options.cycle.cycle_end_at)
      )
      form.setValue('earlyReclaim', true)
      form.setValue('reclaimCapPercent', '100')
      form.setValue('usageBonusPercent', '0')
      return
    }
    if (planType === 'initialization') {
      form.setValue('stagePercent', '0')
      return
    }
    if (stagePercent === '0') {
      const defaultStage = props.options?.defaults?.stage_percent || 7500
      form.setValue(
        'stagePercent',
        String(defaultStage) as FormValues['stagePercent']
      )
      return
    }
    const next = props.options?.schedule?.events.find(
      (event) =>
        event.percent * 100 > Number(stagePercent) &&
        event.time > Date.now() / 1000
    )
    if (props.options?.cycle) {
      form.setValue(
        'nextAdjustmentAt',
        toShanghaiDateInput(next?.time ?? props.options.cycle.cycle_end_at)
      )
    }
  }, [form, planType, props.options, stagePercent, thorough])

  const submit = form.handleSubmit((values) => {
    if (!props.options?.cycle) return
    mutation.mutate(planWriteFromForm(values, props.options.cycle))
  })
  const compare = form.handleSubmit((values) => {
    if (!props.options?.cycle) return
    shadowMutation.mutate(planWriteFromForm(values, props.options.cycle))
  })

  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      <DialogContent className='max-h-[90vh] overflow-y-auto sm:max-w-4xl'>
        <DialogHeader>
          <DialogTitle>{t('Generate quota plan')}</DialogTitle>
          <DialogDescription>
            {t(
              'Review the server defaults before creating an immutable draft.'
            )}
          </DialogDescription>
        </DialogHeader>
        {!props.options?.cycle && (
          <Alert variant='destructive'>
            <AlertTitle>{t('No active cycle')}</AlertTitle>
            <AlertDescription>
              {t(
                'Wait for a scheduled cycle to start before generating a plan.'
              )}
            </AlertDescription>
          </Alert>
        )}
        {props.options?.cycle &&
          thorough &&
          props.options.initialization_required && (
            <Alert variant='destructive'>
              <AlertTitle>{t('Initialization required')}</AlertTitle>
              <AlertDescription>
                {t('Execute an initialization plan before a thorough release.')}
              </AlertDescription>
            </Alert>
          )}
        {props.options?.cycle &&
          !(thorough && props.options.initialization_required) && (
            <form id='generate-quota-plan' onSubmit={submit}>
              <FieldGroup>
                <Alert>
                  <AlertTitle>
                    {t('Cycle')} #{props.options.cycle.id}
                  </AlertTitle>
                  <AlertDescription>
                    {formatDateTime(props.options.cycle.cycle_start_at)} –{' '}
                    {formatDateTime(props.options.cycle.cycle_end_at)}
                    <br />
                    {t('Budget')}:{' '}
                    {formatQuota(props.options.cycle.budget_quota)} ·{' '}
                    {t('Initial grant per user')}:{' '}
                    {formatQuota(props.options.cycle.initial_grant_quota)}
                    {usesCycleLimit && (
                      <>
                        <br />
                        {t('Automatic allocation limit')}:{' '}
                        {formatConcentrationMultiplier(concentrationMultiplier)}{' '}
                        ·{' '}
                        {props.options.initialization_required
                          ? t('Initial grants do not use this limit')
                          : t(
                              'Fixed for this cycle and cannot be changed in a draft'
                            )}
                      </>
                    )}
                    {props.options.schedule && (
                      <>
                        <br />
                        {t('Stage')}: {props.options.schedule.current.percent}%
                        · {t('Next adjustment')}:{' '}
                        {formatDateTime(props.options.schedule.next.time)}
                      </>
                    )}
                  </AlertDescription>
                </Alert>
                {props.options.initialization_required && (
                  <Alert>
                    <AlertTitle>{t('Initialization required')}</AlertTitle>
                    <AlertDescription>
                      {t(
                        'The first executable plan in this cycle must initialize user balances.'
                      )}
                    </AlertDescription>
                  </Alert>
                )}
                <Field
                  orientation='horizontal'
                  data-disabled={props.options.initialization_required}
                >
                  <Controller
                    name='thorough'
                    control={form.control}
                    render={({ field }) => (
                      <Checkbox
                        id='quota-thorough-release'
                        checked={field.value}
                        disabled={props.options?.initialization_required}
                        onCheckedChange={(checked) => {
                          const enabled = checked === true
                          field.onChange(enabled)
                          if (enabled || !props.options?.defaults) return
                          const defaults = props.options.defaults
                          form.setValue('planType', defaults.plan_type)
                          form.setValue(
                            'stagePercent',
                            String(
                              defaults.stage_percent
                            ) as FormValues['stagePercent']
                          )
                          form.setValue(
                            'nextAdjustmentAt',
                            toShanghaiDateInput(defaults.next_adjustment_at)
                          )
                          form.setValue('earlyReclaim', defaults.early_reclaim)
                          form.setValue(
                            'reclaimCapPercent',
                            String(defaults.reclaim_cap_percent)
                          )
                          form.setValue(
                            'usageBonusPercent',
                            String(defaults.usage_bonus_percent)
                          )
                        }}
                      />
                    )}
                  />
                  <FieldContent>
                    <FieldLabel htmlFor='quota-thorough-release'>
                      {t('Thorough quota release')}
                    </FieldLabel>
                  </FieldContent>
                </Field>
                {thorough && (
                  <Alert variant='destructive'>
                    <AlertTitle>{t('Thorough quota release')}</AlertTitle>
                    <AlertDescription>
                      {t(
                        'Use thorough release only when no more than 3 workdays remain in the cycle.'
                      )}
                    </AlertDescription>
                  </Alert>
                )}
                <div className='grid gap-4 sm:grid-cols-2'>
                  <Field>
                    <FieldLabel>{t('Plan type')}</FieldLabel>
                    <Controller
                      name='planType'
                      control={form.control}
                      render={({ field }) => (
                        <Select
                          value={field.value}
                          onValueChange={(value) => field.onChange(value)}
                          disabled={
                            props.options?.initialization_required || thorough
                          }
                        >
                          <SelectTrigger className='w-full'>
                            <SelectValue />
                          </SelectTrigger>
                          <SelectContent>
                            <SelectGroup>
                              <SelectItem value='initialization'>
                                {t('Initialization')}
                              </SelectItem>
                              <SelectItem value='adjustment'>
                                {t('Regular adjustment')}
                              </SelectItem>
                            </SelectGroup>
                          </SelectContent>
                        </Select>
                      )}
                    />
                  </Field>
                  <Field>
                    <FieldLabel>{t('Stage')}</FieldLabel>
                    <Controller
                      name='stagePercent'
                      control={form.control}
                      render={({ field }) => (
                        <Select
                          value={field.value}
                          onValueChange={(value) => field.onChange(value)}
                          disabled={planType === 'initialization' || thorough}
                        >
                          <SelectTrigger className='w-full'>
                            <SelectValue />
                          </SelectTrigger>
                          <SelectContent>
                            <SelectGroup>
                              {planType === 'initialization' && (
                                <SelectItem value='0'>
                                  {t('Initialization')}
                                </SelectItem>
                              )}
                              {['7500', '8500', '9500', '10000'].map(
                                (value) => (
                                  <SelectItem key={value} value={value}>
                                    {Number(value) / 100}%
                                  </SelectItem>
                                )
                              )}
                            </SelectGroup>
                          </SelectContent>
                        </Select>
                      )}
                    />
                  </Field>
                </div>
                <div className='grid gap-4 sm:grid-cols-2'>
                  <Field
                    data-invalid={!!form.formState.errors.nextAdjustmentAt}
                  >
                    <FieldLabel htmlFor='quota-next-adjustment'>
                      {t('Next adjustment time')}
                    </FieldLabel>
                    <Input
                      id='quota-next-adjustment'
                      type='date'
                      disabled={thorough}
                      {...form.register('nextAdjustmentAt')}
                    />
                    <FieldError
                      errors={[form.formState.errors.nextAdjustmentAt]}
                    />
                  </Field>
                  <Field>
                    <FieldLabel>{t('Time basis')}</FieldLabel>
                    <Controller
                      name='basisMode'
                      control={form.control}
                      render={({ field }) => (
                        <Select
                          value={field.value}
                          onValueChange={(value) => field.onChange(value)}
                        >
                          <SelectTrigger className='w-full'>
                            <SelectValue />
                          </SelectTrigger>
                          <SelectContent>
                            <SelectGroup>
                              <SelectItem value='actual'>
                                {t('Actual elapsed days')}
                              </SelectItem>
                              <SelectItem value='week'>
                                {t('Weekly basis')}
                              </SelectItem>
                            </SelectGroup>
                          </SelectContent>
                        </Select>
                      )}
                    />
                  </Field>
                </div>
                <div className='grid gap-4 sm:grid-cols-2'>
                  <Field
                    data-invalid={!!form.formState.errors.reclaimCapPercent}
                  >
                    <FieldLabel htmlFor='quota-reclaim-cap'>
                      {t('Reclaim cap (%)')}
                    </FieldLabel>
                    <Input
                      id='quota-reclaim-cap'
                      type='number'
                      min='0'
                      max='100'
                      disabled={thorough}
                      {...form.register('reclaimCapPercent')}
                    />
                    <FieldError
                      errors={[form.formState.errors.reclaimCapPercent]}
                    />
                  </Field>
                  {!usesCycleLimit && (
                    <Field
                      data-invalid={!!form.formState.errors.usageBonusPercent}
                    >
                      <FieldLabel htmlFor='quota-usage-bonus'>
                        {t('Usage bonus (%)')}
                      </FieldLabel>
                      <Input
                        id='quota-usage-bonus'
                        type='number'
                        min='0'
                        max='100'
                        disabled={thorough}
                        {...form.register('usageBonusPercent')}
                      />
                      <FieldError
                        errors={[form.formState.errors.usageBonusPercent]}
                      />
                    </Field>
                  )}
                </div>
                <Field orientation='horizontal' data-disabled={thorough}>
                  <Controller
                    name='earlyReclaim'
                    control={form.control}
                    render={({ field }) => (
                      <Checkbox
                        checked={field.value}
                        onCheckedChange={(checked) =>
                          field.onChange(checked === true)
                        }
                        disabled={thorough}
                      />
                    )}
                  />
                  <FieldContent>
                    <FieldLabel>{t('Enable early reclaim')}</FieldLabel>
                  </FieldContent>
                </Field>
              </FieldGroup>
            </form>
          )}
        {shadowResult && (
          <FairnessShadowResult
            key={shadowResult.snapshot_at}
            result={shadowResult}
            selectedMultiplierBasisPoints={concentrationMultiplier}
          />
        )}
        <DialogFooter>
          <Button
            type='button'
            variant='outline'
            disabled={
              mutation.isPending ||
              shadowMutation.isPending ||
              evidenceMutation.isPending
            }
            onClick={() => props.onOpenChange(false)}
          >
            {t('Cancel')}
          </Button>
          {!usesCycleLimit &&
            shadowResult?.candidate_qualified &&
            shadowParams?.stage_percent === 10_000 &&
            shadowParams.thorough_release && (
              <Button
                type='button'
                variant='outline'
                disabled={
                  mutation.isPending ||
                  shadowMutation.isPending ||
                  evidenceMutation.isPending
                }
                onClick={() => evidenceMutation.mutate(shadowParams)}
              >
                {evidenceMutation.isPending && (
                  <Spinner data-icon='inline-start' />
                )}
                {t('Record gate evidence')}
              </Button>
            )}
          <Button
            type='button'
            variant='outline'
            disabled={
              disabled ||
              planType !== 'adjustment' ||
              props.options?.initialization_required ||
              mutation.isPending ||
              shadowMutation.isPending ||
              evidenceMutation.isPending
            }
            onClick={compare}
          >
            {shadowMutation.isPending && <Spinner data-icon='inline-start' />}
            {t('Compare fairness')}
          </Button>
          <Button
            type='submit'
            form='generate-quota-plan'
            variant={thorough ? 'destructive' : 'default'}
            disabled={
              disabled ||
              mutation.isPending ||
              shadowMutation.isPending ||
              evidenceMutation.isPending
            }
          >
            {mutation.isPending && <Spinner data-icon='inline-start' />}
            {t('Generate draft')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
