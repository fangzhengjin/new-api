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
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Badge } from '@/components/ui/badge'
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
import { Field, FieldGroup, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Spinner } from '@/components/ui/spinner'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Textarea } from '@/components/ui/textarea'
import type { QuotaRecoveryRequest } from '@/features/quota-recovery/types'
import { getCurrencyDisplay, getCurrencyLabel } from '@/lib/currency'
import { handleServerError } from '@/lib/handle-server-error'

import {
  approveRecoveryRequest,
  listRecoveryRequests,
  rejectRecoveryRequest,
} from '../api'
import {
  formatDateTime,
  quotaFromDisplayAmount,
  quotaToDisplayAmount,
  queryKeys,
} from '../utils'
import { PageError, PageLoading } from './shared'

type ReviewAction = 'approve' | 'reject'

export function RecoveryRequestsCard() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [selected, setSelected] = useState<QuotaRecoveryRequest | null>(null)
  const [action, setAction] = useState<ReviewAction>('approve')
  const [amount, setAmount] = useState('')
  const [reason, setReason] = useState('')
  const { meta } = getCurrencyDisplay()
  const currencyLabel = getCurrencyLabel()
  const requestsQuery = useQuery({
    queryKey: queryKeys.recoveryRequests,
    queryFn: listRecoveryRequests,
  })
  const refresh = async () => {
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: queryKeys.recoveryRequests }),
      queryClient.invalidateQueries({ queryKey: queryKeys.plans }),
      queryClient.invalidateQueries({ queryKey: queryKeys.cycles }),
      queryClient.invalidateQueries({ queryKey: queryKeys.algorithm }),
    ])
  }
  const closeReview = () => {
    setSelected(null)
    setAmount('')
    setReason('')
  }
  const mutation = useMutation({
    mutationFn: async () => {
      if (!selected) throw new Error(t('Select a recovery request'))
      const cleanReason = reason.trim()
      if (!cleanReason || cleanReason.length > 500) {
        throw new Error(t('Enter a review reason up to 500 characters'))
      }
      if (action === 'reject') {
        return rejectRecoveryRequest(selected.id, cleanReason)
      }
      const approvedQuota = quotaFromDisplayAmount(amount)
      if (
        !approvedQuota ||
        BigInt(approvedQuota) > BigInt(selected.requested_quota)
      ) {
        throw new Error(t('Approved quota must not exceed requested quota'))
      }
      return approveRecoveryRequest(selected.id, approvedQuota, cleanReason)
    },
    onSuccess: async () => {
      await refresh()
      toast.success(
        action === 'approve'
          ? t('Recovery request approved')
          : t('Recovery request rejected')
      )
      closeReview()
    },
    onError: handleServerError,
  })
  const openReview = (
    request: QuotaRecoveryRequest,
    nextAction: ReviewAction
  ) => {
    setSelected(request)
    setAction(nextAction)
    setAmount(quotaToDisplayAmount(request.requested_quota))
    setReason('')
  }

  let content
  if (requestsQuery.isPending) {
    content = <PageLoading />
  } else if (requestsQuery.isError) {
    content = (
      <PageError
        title={t('Failed to load recovery requests')}
        error={requestsQuery.error}
      />
    )
  } else if (requestsQuery.data.length === 0) {
    content = (
      <div className='text-muted-foreground py-6 text-center text-sm'>
        {t('No recovery requests')}
      </div>
    )
  } else {
    content = (
      <div className='max-h-96 overflow-auto rounded-xl border'>
        <Table>
          <TableHeader className='bg-background sticky top-0 z-10'>
            <TableRow>
              <TableHead>{t('User')}</TableHead>
              <TableHead>{t('Requested quota')}</TableHead>
              <TableHead>{t('Reason')}</TableHead>
              <TableHead>{t('Status')}</TableHead>
              <TableHead className='text-right'>{t('Actions')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {requestsQuery.data.map((request) => {
              let statusLabel = t('Rejected')
              if (request.status === 'pending') {
                statusLabel = t('Pending review')
              }
              if (request.status === 'executed') statusLabel = t('Granted')
              return (
                <TableRow key={request.id}>
                  <TableCell className='max-w-48 whitespace-normal'>
                    <div className='font-medium break-words'>
                      {request.display_name || request.username}
                    </div>
                    <div className='text-muted-foreground'>
                      #{request.user_id} · {formatDateTime(request.created_at)}
                    </div>
                  </TableCell>
                  <TableCell>
                    {quotaToDisplayAmount(request.requested_quota)}{' '}
                    {currencyLabel}
                  </TableCell>
                  <TableCell className='max-w-64 break-words whitespace-normal'>
                    {request.reason}
                    {request.review_reason && (
                      <div className='text-muted-foreground mt-1'>
                        {request.review_reason}
                      </div>
                    )}
                  </TableCell>
                  <TableCell>
                    <Badge variant='outline'>{statusLabel}</Badge>
                  </TableCell>
                  <TableCell className='text-right'>
                    {request.status === 'pending' && (
                      <div className='flex justify-end gap-2'>
                        <Button
                          size='sm'
                          onClick={() => openReview(request, 'approve')}
                        >
                          {t('Approve')}
                        </Button>
                        <Button
                          size='sm'
                          variant='outline'
                          onClick={() => openReview(request, 'reject')}
                        >
                          {t('Reject')}
                        </Button>
                      </div>
                    )}
                  </TableCell>
                </TableRow>
              )
            })}
          </TableBody>
        </Table>
      </div>
    )
  }

  return (
    <>
      <Card data-card-hover='false'>
        <CardHeader>
          <CardTitle>{t('Temporary recovery review')}</CardTitle>
          <CardDescription>
            {t('Approve only the continuity quota justified by the request.')}
          </CardDescription>
        </CardHeader>
        <CardContent>{content}</CardContent>
      </Card>
      <Dialog
        open={selected !== null}
        onOpenChange={(open) => {
          if (!open && !mutation.isPending) closeReview()
        }}
      >
        <DialogContent className='sm:max-w-md'>
          <DialogHeader>
            <DialogTitle>
              {action === 'approve'
                ? t('Approve recovery request')
                : t('Reject recovery request')}
            </DialogTitle>
            <DialogDescription>
              {selected &&
                t('Requested by {{user}}: {{amount}} {{currency}}', {
                  user: selected.display_name || selected.username,
                  amount: quotaToDisplayAmount(selected.requested_quota),
                  currency: currencyLabel,
                })}
            </DialogDescription>
          </DialogHeader>
          <FieldGroup>
            {action === 'approve' && (
              <Field>
                <FieldLabel htmlFor='recovery-approved-amount'>
                  {t('Approved quota')} ({currencyLabel})
                </FieldLabel>
                <Input
                  id='recovery-approved-amount'
                  inputMode={meta.kind === 'tokens' ? 'numeric' : 'decimal'}
                  value={amount}
                  onChange={(event) => setAmount(event.target.value)}
                  disabled={mutation.isPending}
                />
              </Field>
            )}
            <Field>
              <FieldLabel htmlFor='recovery-review-reason'>
                {t('Review reason')}
              </FieldLabel>
              <Textarea
                id='recovery-review-reason'
                maxLength={500}
                value={reason}
                onChange={(event) => setReason(event.target.value)}
                disabled={mutation.isPending}
              />
            </Field>
          </FieldGroup>
          <DialogFooter>
            <Button
              variant='outline'
              onClick={closeReview}
              disabled={mutation.isPending}
            >
              {t('Cancel')}
            </Button>
            <Button
              variant={action === 'reject' ? 'destructive' : 'default'}
              onClick={() => mutation.mutate()}
              disabled={
                mutation.isPending ||
                !reason.trim() ||
                (action === 'approve' && !amount.trim())
              }
            >
              {mutation.isPending && <Spinner data-icon='inline-start' />}
              {action === 'approve' ? t('Approve and grant') : t('Reject')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )
}
