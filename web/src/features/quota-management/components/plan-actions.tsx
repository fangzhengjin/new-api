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
import { Delete02Icon, PlayIcon, RefreshIcon } from '@hugeicons/core-free-icons'
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
  deletePlan,
  executePlan,
  regeneratePlan,
  retryNotifications,
} from '../api'
import type { PlanDetailData } from '../types'
import { queryKeys } from '../utils'

export function PlanActions(props: { detail: PlanDetailData }) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const [executeOpen, setExecuteOpen] = useState(false)
  const [cancelOpen, setCancelOpen] = useState(false)
  const [deleteOpen, setDeleteOpen] = useState(false)
  const [regenerateOpen, setRegenerateOpen] = useState(false)
  const [confirmation, setConfirmation] = useState('')
  const [reason, setReason] = useState('')

  const refresh = async () => {
    await Promise.all([
      queryClient.invalidateQueries({
        queryKey: queryKeys.plan(props.detail.plan.id),
      }),
      queryClient.invalidateQueries({ queryKey: queryKeys.plans }),
      queryClient.invalidateQueries({
        queryKey: queryKeys.cycle(props.detail.cycle.id),
      }),
      queryClient.invalidateQueries({ queryKey: queryKeys.options }),
    ])
  }
  const executeMutation = useMutation({
    mutationFn: () => executePlan(props.detail.plan.id, confirmation),
    onSuccess: async () => {
      await refresh()
      setExecuteOpen(false)
      setConfirmation('')
      toast.success(t('Quota plan executed'))
    },
    onError: handleServerError,
  })
  const cancelMutation = useMutation({
    mutationFn: () => cancelPlan(props.detail.plan.id, reason.trim()),
    onSuccess: async () => {
      await refresh()
      setCancelOpen(false)
      setReason('')
      toast.success(t('Quota plan cancelled'))
    },
    onError: handleServerError,
  })
  const deleteMutation = useMutation({
    mutationFn: () => deletePlan(props.detail.plan.id),
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: queryKeys.plans }),
        queryClient.invalidateQueries({
          queryKey: queryKeys.plan(props.detail.plan.id),
        }),
        queryClient.invalidateQueries({
          queryKey: queryKeys.cycle(props.detail.cycle.id),
        }),
      ])
      toast.success(t('Quota plan deleted'))
      await navigate({
        to: '/quota-management/cycles/$cycleId',
        params: { cycleId: String(props.detail.cycle.id) },
      })
    },
    onError: handleServerError,
  })
  const regenerateMutation = useMutation({
    mutationFn: () => regeneratePlan(props.detail.plan.id),
    onSuccess: async ({ plan_id }) => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: queryKeys.plans }),
        queryClient.invalidateQueries({
          queryKey: queryKeys.plan(props.detail.plan.id),
        }),
        queryClient.invalidateQueries({
          queryKey: queryKeys.cycle(props.detail.cycle.id),
        }),
      ])
      toast.success(t('Quota plan regenerated'))
      await navigate({
        to: '/quota-management/plans/$planId',
        params: { planId: String(plan_id) },
      })
    },
    onError: handleServerError,
  })
  const retryMutation = useMutation({
    mutationFn: () => retryNotifications(props.detail.plan.id),
    onSuccess: async (result) => {
      await refresh()
      toast.success(
        t(
          'Notification retry completed: {{logs}} logs, {{emails}} emails sent',
          {
            logs: result.logs_sent,
            emails: result.emails_sent,
          }
        )
      )
    },
    onError: handleServerError,
  })

  const isDraft = props.detail.plan.status === 'draft'
  const canDelete = props.detail.plan.status !== 'executed'
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
          {t('Execute plan')}
        </Button>
      )}
      {isDraft && (
        <Button variant='outline' onClick={() => setCancelOpen(true)}>
          {t('Cancel plan')}
        </Button>
      )}
      {canDelete && (
        <Button variant='destructive' onClick={() => setDeleteOpen(true)}>
          <HugeiconsIcon
            icon={Delete02Icon}
            strokeWidth={2}
            data-icon='inline-start'
          />
          {t('Delete')}
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
            onClick={() => retryMutation.mutate()}
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
            {t('Retry notifications')}
          </Button>
        )}

      <Dialog open={executeOpen} onOpenChange={setExecuteOpen}>
        <DialogContent className='sm:max-w-md'>
          <DialogHeader>
            <DialogTitle>{t('Execute this quota plan?')}</DialogTitle>
            <DialogDescription>
              {t(
                'User balances change atomically. Notifications are delivered after the transaction commits.'
              )}
            </DialogDescription>
          </DialogHeader>
          <FieldGroup>
            <Field
              data-invalid={
                confirmation.length > 0 &&
                confirmation !== props.detail.confirmation_phrase
              }
            >
              <FieldLabel htmlFor='execute-plan-confirmation'>
                {t('Enter the exact confirmation phrase')}
              </FieldLabel>
              <Input
                id='execute-plan-confirmation'
                value={confirmation}
                onChange={(event) => setConfirmation(event.target.value)}
                autoComplete='off'
              />
              <FieldDescription>
                {props.detail.confirmation_phrase}
              </FieldDescription>
              {confirmation.length > 0 &&
                confirmation !== props.detail.confirmation_phrase && (
                  <FieldError>
                    {t('Confirmation phrase does not match')}
                  </FieldError>
                )}
            </Field>
          </FieldGroup>
          <DialogFooter>
            <Button
              variant='outline'
              disabled={executeMutation.isPending}
              onClick={() => setExecuteOpen(false)}
            >
              {t('Cancel')}
            </Button>
            <Button
              variant='destructive'
              disabled={
                confirmation !== props.detail.confirmation_phrase ||
                executeMutation.isPending
              }
              onClick={() => executeMutation.mutate()}
            >
              {executeMutation.isPending && (
                <Spinner data-icon='inline-start' />
              )}
              {t('Execute now')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={cancelOpen} onOpenChange={setCancelOpen}>
        <DialogContent className='sm:max-w-md'>
          <DialogHeader>
            <DialogTitle>{t('Cancel this quota plan?')}</DialogTitle>
            <DialogDescription>
              {t(
                'The reason is retained with the cancelled draft for auditing.'
              )}
            </DialogDescription>
          </DialogHeader>
          <Field data-invalid={reason.length > 0 && reason.trim().length < 5}>
            <FieldLabel htmlFor='cancel-plan-reason'>
              {t('Cancellation reason')}
            </FieldLabel>
            <Textarea
              id='cancel-plan-reason'
              value={reason}
              onChange={(event) => setReason(event.target.value)}
            />
            {reason.length > 0 && reason.trim().length < 5 && (
              <FieldError>{t('Enter at least 5 characters')}</FieldError>
            )}
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
              disabled={reason.trim().length < 5 || cancelMutation.isPending}
              onClick={() => cancelMutation.mutate()}
            >
              {cancelMutation.isPending && <Spinner data-icon='inline-start' />}
              {t('Cancel plan')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <AlertDialog open={deleteOpen} onOpenChange={setDeleteOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t('Delete this quota plan?')}</AlertDialogTitle>
            <AlertDialogDescription>
              {t(
                'The draft and every adjustment item will be permanently removed.'
              )}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={deleteMutation.isPending}>
              {t('Cancel')}
            </AlertDialogCancel>
            <AlertDialogAction
              variant='destructive'
              disabled={deleteMutation.isPending}
              onClick={() => deleteMutation.mutate()}
            >
              {deleteMutation.isPending && <Spinner data-icon='inline-start' />}
              {t('Delete')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      <AlertDialog open={regenerateOpen} onOpenChange={setRegenerateOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              {t('Regenerate this quota plan?')}
            </AlertDialogTitle>
            <AlertDialogDescription>
              {t(
                'A new draft uses the saved parameters and a fresh snapshot. The current draft is cancelled.'
              )}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={regenerateMutation.isPending}>
              {t('Cancel')}
            </AlertDialogCancel>
            <AlertDialogAction
              disabled={regenerateMutation.isPending}
              onClick={() => regenerateMutation.mutate()}
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
