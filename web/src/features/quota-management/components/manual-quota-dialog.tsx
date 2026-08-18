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
import { useEffect } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
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
import { Textarea } from '@/components/ui/textarea'

import { manualQuotaAdjust } from '../api'
import { queryKeys } from '../utils'

type Values = { userId: string; targetQuota: string; reason: string }

export function ManualQuotaDialog(props: {
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const form = useForm<Values>({
    defaultValues: { userId: '', targetQuota: '', reason: '' },
  })
  useEffect(() => {
    if (props.open) form.reset({ userId: '', targetQuota: '', reason: '' })
  }, [form, props.open])
  const mutation = useMutation({
    mutationFn: manualQuotaAdjust,
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: queryKeys.cycles })
      toast.success(t('Quota adjustment executed'))
      props.onOpenChange(false)
    },
    onError: (error: Error) => toast.error(error.message),
  })
  const submit = form.handleSubmit((values) => {
    const userId = Number(values.userId)
    if (
      !Number.isInteger(userId) ||
      userId <= 0 ||
      !/^\d+$/.test(values.targetQuota) ||
      !values.reason.trim()
    ) {
      toast.error(
        t('Enter a user ID, a non-negative target quota, and a reason')
      )
      return
    }
    mutation.mutate({
      user_id: userId,
      target_quota: values.targetQuota,
      reason: values.reason.trim(),
    })
  })
  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      <DialogContent className='sm:max-w-md'>
        <DialogHeader>
          <DialogTitle>{t('Manual quota adjustment')}</DialogTitle>
          <DialogDescription>
            {t(
              'Set one managed user target balance. Whitelist users are excluded.'
            )}
          </DialogDescription>
        </DialogHeader>
        <form onSubmit={submit} className='space-y-4'>
          <FieldGroup>
            <Field>
              <FieldLabel htmlFor='manual-quota-user'>
                {t('User ID')}
              </FieldLabel>
              <Input id='manual-quota-user' {...form.register('userId')} />
            </Field>
            <Field>
              <FieldLabel htmlFor='manual-quota-target'>
                {t('Target quota')}
              </FieldLabel>
              <Input
                id='manual-quota-target'
                inputMode='numeric'
                {...form.register('targetQuota')}
              />
            </Field>
            <Field>
              <FieldLabel htmlFor='manual-quota-reason'>
                {t('Reason')}
              </FieldLabel>
              <Textarea
                id='manual-quota-reason'
                maxLength={255}
                {...form.register('reason')}
              />
            </Field>
          </FieldGroup>
          <DialogFooter>
            <Button type='submit' disabled={mutation.isPending}>
              {t('Apply adjustment')}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
