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
import { PlayIcon, RefreshIcon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
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

import {
  cancelPlan,
  executePlan,
  regeneratePlan,
  retryNotifications,
} from '../api'
import type { PlanDetailData, PlanExecuteResult } from '../types'
import { formatQuota, queryKeys } from '../utils'

export function PlanActions(props: { detail: PlanDetailData }) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const [executeOpen, setExecuteOpen] = useState(false)
  const [cancelOpen, setCancelOpen] = useState(false)
  const [regenerateOpen, setRegenerateOpen] = useState(false)
  const [confirmation, setConfirmation] = useState('')
  const [reason, setReason] = useState('')
  const [executionResult, setExecutionResult] =
    useState<PlanExecuteResult | null>(null)

  const refresh = async () => {
    await Promise.all([
      queryClient.invalidateQueries({
        queryKey: queryKeys.plan(props.detail.plan.id),
      }),
      queryClient.invalidateQueries({
        queryKey: queryKeys.cycle(props.detail.cycle.id),
      }),
      queryClient.invalidateQueries({ queryKey: queryKeys.options }),
      queryClient.invalidateQueries({ queryKey: queryKeys.overview }),
    ])
  }
  const executeMutation = useMutation({
    mutationFn: () => executePlan(props.detail.plan.id, confirmation),
    onSuccess: async (result) => {
      await refresh()
      setExecutionResult(result)
      setConfirmation('')
      toast.success(t('额度方案已执行'))
    },
    onError: (error: unknown) => handleServerError(error),
  })
  const closeExecute = () => {
    if (executeMutation.isPending) return
    setExecuteOpen(false)
    setConfirmation('')
    setExecutionResult(null)
  }
  const cancelMutation = useMutation({
    mutationFn: () => cancelPlan(props.detail.plan.id, reason.trim()),
    onSuccess: async () => {
      await refresh()
      setCancelOpen(false)
      setReason('')
      toast.success(t('额度方案已取消'))
    },
    onError: (error: unknown) => handleServerError(error),
  })
  const regenerateMutation = useMutation({
    mutationFn: () => regeneratePlan(props.detail.plan.id),
    onSuccess: async ({ plan_id }) => {
      await Promise.all([
        queryClient.invalidateQueries({
          queryKey: queryKeys.plan(props.detail.plan.id),
        }),
        queryClient.invalidateQueries({
          queryKey: queryKeys.cycle(props.detail.cycle.id),
        }),
      ])
      toast.success(t('额度方案已重新生成'))
      await navigate({
        to: '/quota-management/plans/$planId',
        params: { planId: String(plan_id) },
      })
    },
    onError: (error: unknown) => handleServerError(error),
  })
  const retryMutation = useMutation({
    mutationFn: () => retryNotifications(props.detail.plan.id),
    onSuccess: async (result) => {
      await refresh()
      toast.success(
        t('通知重试完成：已发送 {{logs}} 条日志、{{emails}} 封邮件',
          {
            logs: result.logs_sent,
            emails: result.emails_sent,
          }
        )
      )
    },
    onError: (error: unknown) => handleServerError(error),
  })

  const isDraft = props.detail.plan.status === 'draft'
  const confirmationInvalid =
    confirmation.length > 0 && confirmation !== props.detail.confirmation_phrase
  const hasIncompleteNotifications = props.detail.items.some(
    (item) =>
      item.log_status === 'pending' ||
      item.log_status === 'failed' ||
      item.email_status === 'pending' ||
      item.email_status === 'failed'
  )

  return (
    <>
      {isDraft && (
        <Button onClick={() => setExecuteOpen(true)}>
          <HugeiconsIcon
            icon={PlayIcon}
            strokeWidth={2}
            data-icon='inline-start'
          />
          {t('执行草稿')}
        </Button>
      )}
      {isDraft && (
        <Button variant='outline' onClick={() => setCancelOpen(true)}>
          {t('取消方案')}
        </Button>
      )}
      {props.detail.plan.status !== 'executed' && (
        <Button variant='outline' onClick={() => setRegenerateOpen(true)}>
          <HugeiconsIcon
            icon={RefreshIcon}
            strokeWidth={2}
            data-icon='inline-start'
          />
          {t('Regenerate')}
        </Button>
      )}
      {props.detail.plan.status === 'executed' &&
        hasIncompleteNotifications && (
          <Button
            variant='outline'
            disabled={retryMutation.isPending}
            onClick={() => retryMutation.mutate(undefined)}
          >
            {retryMutation.isPending ? (
              <Spinner data-icon='inline-start' />
            ) : (
              <HugeiconsIcon
                icon={RefreshIcon}
                strokeWidth={2}
                data-icon='inline-start'
              />
            )}
            {t('重试通知')}
          </Button>
        )}

      <Dialog
        open={executeOpen}
        onOpenChange={(open) => {
          if (open) {
            setExecuteOpen(true)
          } else {
            closeExecute()
          }
        }}
      >
        <DialogContent className='sm:max-w-md'>
          {executionResult ? (
            <>
              <DialogHeader>
                <DialogTitle>{t('方案已执行')}</DialogTitle>
                <DialogDescription>
                  {t('以下额度为实际执行结果')}
                </DialogDescription>
              </DialogHeader>
              <div className='grid gap-2 sm:grid-cols-2'>
                {[
                  [t('额度发放'), executionResult.issued_quota],
                  [t('额度回收'), executionResult.reclaimed_quota],
                  [t('已分配额度'), executionResult.allocated_quota],
                  [t('周期剩余额度'), executionResult.remaining_quota],
                ].map(([label, value]) => (
                  <div key={label} className='bg-muted/40 rounded-lg p-3'>
                    <div className='text-muted-foreground text-xs'>{label}</div>
                    <div className='mt-1 font-medium tabular-nums'>
                      {formatQuota(value)}
                    </div>
                  </div>
                ))}
              </div>
              <DialogFooter>
                <Button onClick={closeExecute}>{t('Done')}</Button>
              </DialogFooter>
            </>
          ) : (
            <>
              <DialogHeader>
                <DialogTitle>{t('是否执行此分配方案')}</DialogTitle>
                <DialogDescription>
                  {t('列出的调整会作用于用户当前余额。普通余额变化不会使草稿失效。若计划调减会导致余额为负，或用户、周期、分配规则已不再符合条件，系统将停止执行')}
                </DialogDescription>
              </DialogHeader>
              <div className='grid grid-cols-2 gap-2'>
                <div className='bg-muted/40 rounded-lg p-3'>
                  <div className='text-muted-foreground text-xs'>
                    {t('方案发放')}
                  </div>
                  <div className='mt-1 font-medium'>
                    {formatQuota(props.detail.summary.increase.total)}
                  </div>
                </div>
                <div className='bg-muted/40 rounded-lg p-3'>
                  <div className='text-muted-foreground text-xs'>
                    {t('方案回收')}
                  </div>
                  <div className='mt-1 font-medium'>
                    {formatQuota(props.detail.summary.decrease.total)}
                  </div>
                </div>
              </div>
              <FieldGroup>
                <Field data-invalid={confirmationInvalid}>
                  <FieldLabel htmlFor='execute-plan-confirmation'>
                    {t('请输入完整且一致的确认短语')}
                  </FieldLabel>
                  <Input
                    id='execute-plan-confirmation'
                    aria-invalid={confirmationInvalid}
                    aria-describedby={
                      confirmationInvalid
                        ? 'execute-plan-confirmation-description execute-plan-confirmation-error'
                        : 'execute-plan-confirmation-description'
                    }
                    value={confirmation}
                    onChange={(event) => setConfirmation(event.target.value)}
                    autoComplete='off'
                  />
                  <FieldDescription id='execute-plan-confirmation-description'>
                    {props.detail.confirmation_phrase}
                  </FieldDescription>
                  {confirmationInvalid && (
                    <FieldError id='execute-plan-confirmation-error'>
                      {t('确认短语不匹配')}
                    </FieldError>
                  )}
                </Field>
              </FieldGroup>
              <DialogFooter>
                <Button
                  variant='outline'
                  disabled={executeMutation.isPending}
                  onClick={closeExecute}
                >
                  {t('Cancel')}
                </Button>
                <Button
                  variant='destructive'
                  disabled={
                    confirmation !== props.detail.confirmation_phrase ||
                    executeMutation.isPending
                  }
                  onClick={() => executeMutation.mutate(undefined)}
                >
                  {executeMutation.isPending && (
                    <Spinner data-icon='inline-start' />
                  )}
                  {t('立即执行')}
                </Button>
              </DialogFooter>
            </>
          )}
        </DialogContent>
      </Dialog>

      <Dialog open={cancelOpen} onOpenChange={setCancelOpen}>
        <DialogContent className='sm:max-w-md'>
          <DialogHeader>
            <DialogTitle>{t('取消此额度方案')}</DialogTitle>
            <DialogDescription>
              {t('取消说明会保存在草稿中，方便管理员以后查看')}
            </DialogDescription>
          </DialogHeader>
          <Field>
            <FieldLabel htmlFor='cancel-plan-reason'>
              {t('取消原因')}
            </FieldLabel>
            <Textarea
              id='cancel-plan-reason'
              maxLength={500}
              value={reason}
              onChange={(event) => setReason(event.target.value)}
            />
          </Field>
          <DialogFooter>
            <Button
              variant='outline'
              disabled={cancelMutation.isPending}
              onClick={() => setCancelOpen(false)}
            >
              {t('Back')}
            </Button>
            <Button
              variant='destructive'
              disabled={!reason.trim() || cancelMutation.isPending}
              onClick={() => cancelMutation.mutate(undefined)}
            >
              {cancelMutation.isPending && <Spinner data-icon='inline-start' />}
              {t('取消方案')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <AlertDialog open={regenerateOpen} onOpenChange={setRegenerateOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              {t('按最新数据重新计算这份草稿')}
            </AlertDialogTitle>
            <AlertDialogDescription>
              {t('当前草稿会被取消，新草稿沿用相同设置，并按最新余额和使用情况重新计算')}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={regenerateMutation.isPending}>
              {t('Cancel')}
            </AlertDialogCancel>
            <AlertDialogAction
              disabled={regenerateMutation.isPending}
              onClick={() => regenerateMutation.mutate(undefined)}
            >
              {regenerateMutation.isPending && (
                <Spinner data-icon='inline-start' />
              )}
              {t('Regenerate')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  )
}
