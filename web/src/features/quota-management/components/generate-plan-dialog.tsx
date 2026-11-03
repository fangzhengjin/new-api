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
          .min(1, t('方案名称为必填项'))
          .max(64, t('方案名称不能超过 64 个字符')),
        purpose: z
          .string()
          .trim()
          .min(1, t('方案用途为必填项'))
          .max(200, t('方案用途不能超过 200 个字符')),
        nextAdjustmentAt: z
          .string()
          .min(1, t('请选择下次调配时间')),
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
        ? t('初始分配')
        : '',
      purpose: props.options.initialization_required
        ? t('设置受管用户的初始额度')
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
        queryClient.invalidateQueries({ queryKey: queryKeys.options }),
        queryClient.invalidateQueries({
          queryKey: queryKeys.cycle(props.options?.cycle?.id ?? 0),
        }),
      ])
      toast.success(t('调配草稿已生成'))
      props.onOpenChange(false)
      await navigate({
        to: '/quota-management/plans/$planId',
        params: { planId: String(plan_id) },
      })
    },
    onError: (error: unknown) => handleServerError(error),
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
        message: t('下次调整时间必须晚于当前时间，且在周期范围内'),
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
          <DialogTitle>{t('创建分配方案')}</DialogTitle>
          <DialogDescription>
            {t('为额度调配方案命名并选择下次复核时间，执行前可在草稿中查看全部计划发放和回收')}
          </DialogDescription>
        </DialogHeader>
        {!props.options?.cycle && (
          <Alert variant='destructive'>
            <AlertTitle>{t('没有活跃周期')}</AlertTitle>
            <AlertDescription>
              {t('等已排期周期开始后，再生成调配草稿')}
            </AlertDescription>
          </Alert>
        )}
        {props.options?.cycle && (
          <form id='generate-quota-plan' onSubmit={submit}>
            <FieldGroup>
              <Alert>
                <AlertTitle>
                  {t('当前周期')} #{props.options.cycle.id}
                </AlertTitle>
                <AlertDescription>
                  {formatDateTime(props.options.cycle.cycle_start_at)} –{' '}
                  {formatDateTime(props.options.cycle.cycle_end_at)} ·{' '}
                  {t('周期总额度')}:{' '}
                  {formatQuota(props.options.cycle.budget_quota)}
                </AlertDescription>
              </Alert>
              {props.options.initialization_required && (
                <Alert>
                  <AlertTitle>{t('首次分配')}</AlertTitle>
                  <AlertDescription>
                    {t('这将是本周期的首份方案。系统会计算每位受管用户达到初始额度所需的发放额度')}
                  </AlertDescription>
                </Alert>
              )}
              <Field data-invalid={!!form.formState.errors.name}>
                <FieldLabel htmlFor='quota-plan-name'>
                  {t('方案名称')}
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
                  placeholder={t('示例：新项目分配')}
                  {...form.register('name')}
                />
                <FieldDescription id='quota-plan-name-description'>
                  {t('使用简短名称标识本次额度调配方案')}
                </FieldDescription>
                <FieldError
                  id='quota-plan-name-error'
                  errors={[form.formState.errors.name]}
                />
              </Field>
              <Field data-invalid={!!form.formState.errors.purpose}>
                <FieldLabel htmlFor='quota-plan-purpose'>
                  {t('用途')}
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
                  placeholder={t('说明此分配的必要性')}
                  {...form.register('purpose')}
                />
                <FieldError
                  id='quota-plan-purpose-error'
                  errors={[form.formState.errors.purpose]}
                />
              </Field>
              <Field data-invalid={!!form.formState.errors.nextAdjustmentAt}>
                <FieldLabel htmlFor='quota-next-adjustment'>
                  {t('下次方案调整')}
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
                  {t('这是下次复核时间的提醒，不代表方案的持续时间')}
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
            {t('创建草稿')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
