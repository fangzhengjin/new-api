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

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
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
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Spinner } from '@/components/ui/spinner'
import { handleServerError } from '@/lib/handle-server-error'

import { getQuotaAlgorithmStatus, switchQuotaAlgorithm } from '../api'
import { queryKeys } from '../utils'
import { PageError, PageLoading } from './shared'

export function AlgorithmSwitchCard() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [dialogOpen, setDialogOpen] = useState(false)
  const [confirmation, setConfirmation] = useState('')
  const query = useQuery({
    queryKey: queryKeys.algorithm,
    queryFn: getQuotaAlgorithmStatus,
  })
  const status = query.data
  const rollingBack = status?.current_version === status?.candidate_version
  const expectedConfirmation = rollingBack
    ? (status?.rollback_confirmation_phrase ?? '')
    : (status?.enable_confirmation_phrase ?? '')
  const targetVersion = rollingBack
    ? (status?.legacy_version ?? '')
    : (status?.candidate_version ?? '')
  const mutation = useMutation({
    mutationFn: () => switchQuotaAlgorithm(targetVersion, confirmation.trim()),
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: queryKeys.algorithm }),
        queryClient.invalidateQueries({ queryKey: queryKeys.cycles }),
        queryClient.invalidateQueries({ queryKey: queryKeys.options }),
        queryClient.invalidateQueries({ queryKey: queryKeys.plans }),
      ])
      toast.success(
        rollingBack
          ? t('Current algorithm restored')
          : t('Candidate algorithm enabled')
      )
      setDialogOpen(false)
      setConfirmation('')
    },
    onError: handleServerError,
  })

  if (
    status &&
    status.current_version !== status.legacy_version &&
    status.current_version !== status.candidate_version
  ) {
    return null
  }

  let content
  if (query.isPending) {
    content = <PageLoading />
  } else if (query.isError) {
    content = (
      <PageError
        title={t('Failed to load algorithm status')}
        error={query.error}
      />
    )
  } else {
    const missingEvidence = Math.max(
      0,
      query.data.required_qualified_cycles -
        query.data.qualified_cycle_ids.length
    )
    content = (
      <div className='space-y-4'>
        <div className='grid gap-3 sm:grid-cols-3'>
          <div className='rounded-xl border p-3'>
            <div className='text-muted-foreground text-sm'>
              {t('Production version')}
            </div>
            <div className='mt-1 font-medium'>{query.data.current_version}</div>
          </div>
          <div className='rounded-xl border p-3'>
            <div className='text-muted-foreground text-sm'>
              {t('Qualified closed cycles')}
            </div>
            <div className='mt-1 font-medium'>
              {query.data.qualified_cycle_ids.length} /{' '}
              {query.data.required_qualified_cycles}
            </div>
          </div>
          <div className='rounded-xl border p-3'>
            <div className='text-muted-foreground text-sm'>
              {t('Pending drafts')}
            </div>
            <div className='mt-1 font-medium'>{query.data.draft_count}</div>
          </div>
        </div>
        {!rollingBack && !query.data.can_switch && (
          <Alert>
            <AlertTitle>{t('Switch gate is not ready')}</AlertTitle>
            <AlertDescription>
              <ul className='list-disc space-y-1 pl-5'>
                {!query.data.active_cycle_id && (
                  <li>{t('An active cycle is required')}</li>
                )}
                {!query.data.recovery_ready && (
                  <li>{t('Configure a positive recovery reserve')}</li>
                )}
                {missingEvidence > 0 && (
                  <li>
                    {t('{{count}} more qualified closed cycles are required', {
                      count: missingEvidence,
                    })}
                  </li>
                )}
                {query.data.draft_count > 0 && (
                  <li>
                    {t('Cancel or execute all {{count}} pending drafts', {
                      count: query.data.draft_count,
                    })}
                  </li>
                )}
              </ul>
            </AlertDescription>
          </Alert>
        )}
        <div className='flex flex-wrap items-center justify-between gap-3'>
          <Badge variant='outline'>
            {query.data.recovery_ready
              ? t('Recovery guard ready')
              : t('Recovery guard missing')}
          </Badge>
          {!rollingBack && (
            <Button
              disabled={!query.data.can_switch}
              onClick={() => setDialogOpen(true)}
            >
              {t('Enable candidate algorithm')}
            </Button>
          )}
          {rollingBack && query.data.rollback_allowed && (
            <Button variant='destructive' onClick={() => setDialogOpen(true)}>
              {t('Roll back during this cycle')}
            </Button>
          )}
        </div>
      </div>
    )
  }

  return (
    <>
      <Card data-card-hover='false'>
        <CardHeader>
          <CardTitle>{t('Allocation algorithm gate')}</CardTitle>
          <CardDescription>
            {t(
              'Production switching requires two qualified closed-cycle shadow results, a recovery reserve, and no drafts.'
            )}
          </CardDescription>
        </CardHeader>
        <CardContent>{content}</CardContent>
      </Card>
      <Dialog
        open={dialogOpen}
        onOpenChange={(open) => {
          if (!open && mutation.isPending) return
          setDialogOpen(open)
          if (!open) setConfirmation('')
        }}
      >
        <DialogContent className='sm:max-w-md'>
          <DialogHeader>
            <DialogTitle>
              {rollingBack
                ? t('Roll back allocation algorithm?')
                : t('Enable candidate allocation algorithm?')}
            </DialogTitle>
            <DialogDescription>
              {rollingBack
                ? t(
                    'Rollback is available only in the switch cycle and cancels candidate drafts.'
                  )
                : t(
                    'The candidate becomes authoritative for the active and scheduled cycles without changing balances.'
                  )}
            </DialogDescription>
          </DialogHeader>
          <Field>
            <FieldLabel htmlFor='algorithm-switch-confirmation'>
              {t('Confirmation phrase')}
            </FieldLabel>
            <Input
              id='algorithm-switch-confirmation'
              value={confirmation}
              onChange={(event) => setConfirmation(event.target.value)}
              disabled={mutation.isPending}
            />
            <FieldDescription>
              {t('Enter exactly: {{phrase}}', {
                phrase: expectedConfirmation,
              })}
            </FieldDescription>
          </Field>
          <DialogFooter>
            <Button
              variant='outline'
              onClick={() => setDialogOpen(false)}
              disabled={mutation.isPending}
            >
              {t('Cancel')}
            </Button>
            <Button
              variant={rollingBack ? 'destructive' : 'default'}
              onClick={() => mutation.mutate()}
              disabled={
                mutation.isPending ||
                confirmation.trim() !== expectedConfirmation
              }
            >
              {mutation.isPending && <Spinner data-icon='inline-start' />}
              {rollingBack ? t('Confirm rollback') : t('Confirm switch')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )
}
