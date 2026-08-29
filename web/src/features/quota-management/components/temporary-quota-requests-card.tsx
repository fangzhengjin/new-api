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

import { StatusBadge } from '@/components/status-badge'
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
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Textarea } from '@/components/ui/textarea'
import { TEMPORARY_QUOTA_REQUEST_STATUSES } from '@/features/temporary-quota/constants'
import type { TemporaryQuotaRequest } from '@/features/temporary-quota/types'
import { getCurrencyDisplay, getCurrencyLabel } from '@/lib/currency'
import { handleServerError } from '@/lib/handle-server-error'

import {
  approveTemporaryQuotaRequest,
  listTemporaryQuotaRequests,
  rejectTemporaryQuotaRequest,
} from '../api'
import {
  formatDateTime,
  quotaFromDisplayAmount,
  quotaToDisplayAmount,
  queryKeys,
} from '../utils'
import { PageError, PageLoading } from './shared'

type ReviewAction = 'approve' | 'reject'

export function TemporaryQuotaRequestsCard() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [selected, setSelected] = useState<TemporaryQuotaRequest | null>(null)
  const [action, setAction] = useState<ReviewAction>('approve')
  const [amount, setAmount] = useState('')
  const [reason, setReason] = useState('')
  const [amountError, setAmountError] = useState('')
  const [reasonError, setReasonError] = useState('')
  const { meta } = getCurrencyDisplay()
  const currencyLabel = getCurrencyLabel()
  const requestsQuery = useQuery({
    queryKey: queryKeys.temporaryQuotaRequests,
    queryFn: listTemporaryQuotaRequests,
  })
  const refresh = async () => {
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: queryKeys.overview }),
      queryClient.invalidateQueries({ queryKey: queryKeys.activities }),
      queryClient.invalidateQueries({
        queryKey: queryKeys.temporaryQuotaRequests,
      }),
      queryClient.invalidateQueries({ queryKey: queryKeys.cycles }),
    ])
  }
  const closeReview = () => {
    setSelected(null)
    setAmount('')
    setReason('')
    setAmountError('')
    setReasonError('')
  }
  const mutation = useMutation({
    mutationFn: async (variables: {
      approvedQuota?: string
      reviewReason: string
    }) => {
      if (!selected) throw new Error(t('Select a temporary quota request'))
      if (action === 'reject') {
        return rejectTemporaryQuotaRequest(selected.id, variables.reviewReason)
      }
      if (!variables.approvedQuota) {
        throw new Error(t('Approved quota must not exceed requested quota'))
      }
      return approveTemporaryQuotaRequest(
        selected.id,
        variables.approvedQuota,
        variables.reviewReason
      )
    },
    onSuccess: async () => {
      await refresh()
      toast.success(
        action === 'approve'
          ? t('Temporary quota approved')
          : t('Temporary quota rejected')
      )
      closeReview()
    },
    onError: handleServerError,
  })
  const openReview = (
    request: TemporaryQuotaRequest,
    nextAction: ReviewAction
  ) => {
    setSelected(request)
    setAction(nextAction)
    setAmount(quotaToDisplayAmount(request.requested_quota))
    setReason('')
    setAmountError('')
    setReasonError('')
  }
  const submitReview = () => {
    if (!selected) return
    const cleanReason = reason.trim()
    const nextReasonError =
      !cleanReason || [...cleanReason].length > 500
        ? t('Enter a review reason up to 500 characters')
        : ''
    let approvedQuota: string | undefined
    let nextAmountError = ''
    if (action === 'approve') {
      approvedQuota = quotaFromDisplayAmount(amount) ?? undefined
      if (
        !approvedQuota ||
        BigInt(approvedQuota) > BigInt(selected.requested_quota)
      ) {
        nextAmountError = t('Approved quota must not exceed requested quota')
      }
    }
    setReasonError(nextReasonError)
    setAmountError(nextAmountError)
    if (nextReasonError || nextAmountError) return
    mutation.mutate({ approvedQuota, reviewReason: cleanReason })
  }

  let content
  if (requestsQuery.isPending) {
    content = <PageLoading />
  } else if (requestsQuery.isError) {
    content = (
      <PageError
        title={t('Failed to load temporary quota requests')}
        error={requestsQuery.error}
      />
    )
  } else if (requestsQuery.data.length === 0) {
    content = (
      <div className='text-muted-foreground py-6 text-center text-sm'>
        {t('No temporary quota requests')}
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
              <TableHead>{t('Project')}</TableHead>
              <TableHead>{t('Reason')}</TableHead>
              <TableHead>{t('Status')}</TableHead>
              <TableHead className='text-right'>{t('Actions')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {requestsQuery.data.map((request) => {
              const status = TEMPORARY_QUOTA_REQUEST_STATUSES[request.status]
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
                  <TableCell className='max-w-48 break-words whitespace-normal'>
                    {request.project}
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
                    <StatusBadge
                      label={t(status.labelKey)}
                      variant={status.variant}
                      copyable={false}
                    />
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
          <CardTitle>{t('Temporary quota requests')}</CardTitle>
          <CardDescription>
            {t(
              'Review temporary quota requested by users with insufficient balance'
            )}
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
                ? t('Approve temporary quota request')
                : t('Reject temporary quota request')}
            </DialogTitle>
            <DialogDescription>
              {selected && (
                <>
                  {t('Requested by {{user}}: {{amount}} {{currency}}', {
                    user: selected.display_name || selected.username,
                    amount: quotaToDisplayAmount(selected.requested_quota),
                    currency: currencyLabel,
                  })}
                  <br />
                  {t('Project')}: {selected.project}
                </>
              )}
            </DialogDescription>
          </DialogHeader>
          <FieldGroup>
            {action === 'approve' && (
              <Field data-invalid={!!amountError}>
                <FieldLabel htmlFor='temporary-quota-approved-amount'>
                  {t('Approved quota')} ({currencyLabel})
                </FieldLabel>
                <Input
                  id='temporary-quota-approved-amount'
                  aria-invalid={!!amountError}
                  aria-describedby={
                    amountError
                      ? 'temporary-quota-approved-amount-error'
                      : undefined
                  }
                  inputMode={meta.kind === 'tokens' ? 'numeric' : 'decimal'}
                  value={amount}
                  onChange={(event) => {
                    setAmount(event.target.value)
                    setAmountError('')
                  }}
                  disabled={mutation.isPending}
                />
                <FieldError id='temporary-quota-approved-amount-error'>
                  {amountError}
                </FieldError>
              </Field>
            )}
            <Field data-invalid={!!reasonError}>
              <FieldLabel htmlFor='temporary-quota-review-reason'>
                {t('Review reason')}
              </FieldLabel>
              <Textarea
                id='temporary-quota-review-reason'
                aria-invalid={!!reasonError}
                aria-describedby={
                  reasonError
                    ? 'temporary-quota-review-reason-error'
                    : undefined
                }
                value={reason}
                onChange={(event) => {
                  setReason(event.target.value)
                  setReasonError('')
                }}
                disabled={mutation.isPending}
              />
              <FieldError id='temporary-quota-review-reason-error'>
                {reasonError}
              </FieldError>
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
              onClick={submitReview}
              disabled={mutation.isPending}
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
