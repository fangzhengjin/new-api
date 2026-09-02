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
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect, useState } from 'react'
import { useForm, useFormContext } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import * as z from 'zod'

import { ConfirmDialog } from '@/components/confirm-dialog'
import {
  SideDrawerSection,
  SideDrawerSectionHeader,
  sideDrawerContentClassName,
  sideDrawerFooterClassName,
  sideDrawerFormClassName,
  sideDrawerHeaderClassName,
} from '@/components/drawer-layout'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import {
  Sheet,
  SheetClose,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { formatTimestamp } from '@/lib/format'
import {
  ACCESS_SOURCE_ASSOCIATION_MAX,
  ACCESS_SOURCE_SWITCH_COOLDOWN_MINUTES_MAX,
  MODEL_REQUEST_CONCURRENCY_MAX,
  MODEL_REQUEST_LIMIT_MAX,
} from '@/lib/request-limits'

import {
  allowLatestUserAccessSource,
  getUserLimits,
  removeUserAccessSource,
  updateUserLimits,
} from '../../api'
import type {
  AccessSourceRejection,
  LimitSource,
  User,
  UserLimitOverrides,
} from '../../types'

const limitSchema = z.object({
  model_request_rate_limit_count: z
    .number()
    .int()
    .min(0)
    .max(MODEL_REQUEST_LIMIT_MAX)
    .nullable(),
  model_request_rate_limit_success_count: z
    .number()
    .int()
    .min(0)
    .max(MODEL_REQUEST_LIMIT_MAX)
    .nullable(),
  model_request_concurrency_limit: z
    .number()
    .int()
    .min(0)
    .max(MODEL_REQUEST_CONCURRENCY_MAX)
    .nullable(),
  access_source_max_ips: z
    .number()
    .int()
    .min(0)
    .max(ACCESS_SOURCE_ASSOCIATION_MAX)
    .nullable(),
  access_source_switch_cooldown_minutes: z
    .number()
    .int()
    .min(0)
    .max(ACCESS_SOURCE_SWITCH_COOLDOWN_MINUTES_MAX)
    .nullable(),
})

type LimitFieldName = keyof UserLimitOverrides
const limitSourceKeys: Record<LimitSource, string> = {
  global: 'Global',
  group: 'Group',
  user: 'User',
}

function LimitOverrideField(props: {
  name: LimitFieldName
  label: string
  description: string
  effectiveValue: number
  source: LimitSource
  max: number
}) {
  const { t } = useTranslation()
  const form = useFormContext<UserLimitOverrides>()
  return (
    <FormField
      control={form.control}
      name={props.name}
      render={({ field }) => (
        <FormItem>
          <div className='flex items-center justify-between gap-2'>
            <FormLabel>{props.label}</FormLabel>
            <Badge variant='secondary'>
              {field.value === null
                ? t('Inherited: {{value}}', { value: props.effectiveValue })
                : t('User override')}
            </Badge>
          </div>
          <FormControl>
            <Input
              type='number'
              min={0}
              max={props.max}
              step={1}
              value={field.value ?? ''}
              placeholder={String(props.effectiveValue)}
              onBlur={field.onBlur}
              name={field.name}
              ref={field.ref}
              onChange={(event) => {
                field.onChange(
                  event.target.value === '' ? null : event.target.valueAsNumber
                )
              }}
            />
          </FormControl>
          <FormDescription>
            {props.description} ·{' '}
            {t('Leave empty to inherit {{source}}', {
              source: t(limitSourceKeys[props.source]),
            })}
          </FormDescription>
          <FormMessage />
        </FormItem>
      )}
    />
  )
}

type PendingAction =
  | { type: 'remove'; ip: string }
  | { type: 'allow'; rejection: AccessSourceRejection }

function rejectionReason(t: (key: string) => string, reason: string) {
  if (reason === 'switch_cooldown') return t('IP switch wait is active')
  if (reason === 'account_ip_limit') return t('Account IP limit reached')
  return t('IP account limit reached')
}

export function UserLimitSettingsDrawer(props: {
  open: boolean
  onOpenChange: (open: boolean) => void
  user: User
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [pendingAction, setPendingAction] = useState<PendingAction | null>(null)
  const queryKey = ['user-limits', props.user.id]
  const limitsQuery = useQuery({
    queryKey,
    enabled: props.open,
    queryFn: async () => {
      const response = await getUserLimits(props.user.id)
      if (!response.success || !response.data) {
        throw new Error(response.message || t('Failed to load limit settings'))
      }
      return response.data
    },
  })
  const form = useForm<UserLimitOverrides>({
    resolver: zodResolver(limitSchema),
    defaultValues: {
      model_request_rate_limit_count: null,
      model_request_rate_limit_success_count: null,
      model_request_concurrency_limit: null,
      access_source_max_ips: null,
      access_source_switch_cooldown_minutes: null,
    },
  })

  useEffect(() => {
    if (limitsQuery.data && !form.formState.isDirty) {
      form.reset(limitsQuery.data.overrides)
    }
  }, [form, form.formState.isDirty, limitsQuery.data])

  const saveMutation = useMutation({
    mutationFn: async (values: UserLimitOverrides) => {
      const response = await updateUserLimits(props.user.id, values)
      if (!response.success) {
        throw new Error(response.message || t('Failed to save limit settings'))
      }
    },
    onSuccess: async (_, values) => {
      form.reset(values)
      await queryClient.invalidateQueries({ queryKey })
      toast.success(t('Limit settings saved'))
    },
    onError: (error: Error) => toast.error(error.message),
  })

  const sourceMutation = useMutation({
    mutationFn: async (action: PendingAction) => {
      const response =
        action.type === 'remove'
          ? await removeUserAccessSource(props.user.id, action.ip)
          : await allowLatestUserAccessSource(
              props.user.id,
              action.rejection.event_id,
              action.rejection.ip
            )
      if (!response.success) {
        throw new Error(response.message || t('Failed to update access source'))
      }
    },
    onSuccess: async () => {
      setPendingAction(null)
      await queryClient.invalidateQueries({ queryKey })
      toast.success(t('Access source updated'))
    },
    onError: (error: Error) => {
      setPendingAction(null)
      toast.error(error.message)
      queryClient.invalidateQueries({ queryKey })
    },
  })

  const data = limitsQuery.data
  const state = data?.access_source_state
  const associations = state?.associations ?? []
  const rejections = state?.recent_rejections ?? []
  const pending = state?.pending

  return (
    <>
      <Sheet open={props.open} onOpenChange={props.onOpenChange}>
        <SheetContent
          className={sideDrawerContentClassName('sm:max-w-[720px]')}
        >
          <SheetHeader className={sideDrawerHeaderClassName()}>
            <SheetTitle>{t('User Limit Settings')}</SheetTitle>
            <SheetDescription>
              {t(
                'Set account-specific limits and manage access sources for {{username}}',
                {
                  username: props.user.username,
                }
              )}
            </SheetDescription>
          </SheetHeader>

          {limitsQuery.isLoading && (
            <div
              className={sideDrawerFormClassName()}
              aria-label={t('Loading')}
            >
              <Skeleton className='h-28 w-full' />
              <Skeleton className='h-48 w-full' />
              <Skeleton className='h-56 w-full' />
            </div>
          )}

          {limitsQuery.isError && (
            <div className={sideDrawerFormClassName('justify-center')}>
              <p className='text-destructive text-sm' role='alert'>
                {limitsQuery.error.message}
              </p>
              <Button variant='outline' onClick={() => limitsQuery.refetch()}>
                {t('Retry')}
              </Button>
            </div>
          )}

          {data && (
            <Form {...form}>
              <form
                id='user-limit-settings-form'
                className={sideDrawerFormClassName()}
                onSubmit={form.handleSubmit((values) =>
                  saveMutation.mutate(values)
                )}
              >
                <SideDrawerSection>
                  <SideDrawerSectionHeader
                    title={t('Account Request Limits')}
                    description={t(
                      'User values override group and global values for this account only'
                    )}
                  />
                  <div className='grid gap-4 sm:grid-cols-2'>
                    <LimitOverrideField
                      name='model_request_rate_limit_count'
                      label={t('Max requests per account per period')}
                      description={t('Includes failed requests, 0 = unlimited')}
                      effectiveValue={
                        data.effective.model_request_rate_limit_count
                      }
                      source={
                        data.effective.model_request_rate_limit_count_source
                      }
                      max={MODEL_REQUEST_LIMIT_MAX}
                    />
                    <LimitOverrideField
                      name='model_request_rate_limit_success_count'
                      label={t(
                        'Max successful requests per account per period'
                      )}
                      description={t('Successful requests only, 0 = unlimited')}
                      effectiveValue={
                        data.effective.model_request_rate_limit_success_count
                      }
                      source={
                        data.effective.model_request_rate_limit_success_source
                      }
                      max={MODEL_REQUEST_LIMIT_MAX}
                    />
                  </div>
                  <p className='text-muted-foreground text-xs'>
                    {data.global.model_request_rate_limit_enabled
                      ? t('Shared period: {{minutes}} minutes', {
                          minutes:
                            data.global
                              .model_request_rate_limit_duration_minutes,
                        })
                      : t('Global request rate limiting is disabled')}
                  </p>
                </SideDrawerSection>

                <SideDrawerSection>
                  <SideDrawerSectionHeader
                    title={t('Concurrency Limits')}
                    description={t(
                      'The account limit can be overridden for this user, while the IP limit remains global'
                    )}
                  />
                  <div className='grid gap-4 sm:grid-cols-2'>
                    <LimitOverrideField
                      name='model_request_concurrency_limit'
                      label={t('Max concurrent requests per account')}
                      description={t(
                        'Counts requests currently being processed, 0 = unlimited'
                      )}
                      effectiveValue={
                        data.effective.model_request_concurrency_limit
                      }
                      source={
                        data.effective.model_request_concurrency_limit_source
                      }
                      max={MODEL_REQUEST_CONCURRENCY_MAX}
                    />
                  </div>
                  <p className='text-muted-foreground text-xs'>
                    {data.global.model_request_concurrency_limit_enabled
                      ? t('Global max concurrent requests per IP: {{count}}', {
                          count: data.global.model_request_ip_concurrency_limit,
                        })
                      : t('Global concurrency limiting is disabled')}
                  </p>
                </SideDrawerSection>

                <SideDrawerSection>
                  <SideDrawerSectionHeader
                    title={t('Access Source Limits')}
                    description={t(
                      'User values override global account rules; the per-IP account limit remains global'
                    )}
                  />
                  <div className='grid gap-4 sm:grid-cols-2'>
                    <LimitOverrideField
                      name='access_source_max_ips'
                      label={t('Max IPs per account')}
                      description={t('0 = unlimited')}
                      effectiveValue={data.effective.access_source_max_ips}
                      source={data.effective.access_source_max_ips_source}
                      max={ACCESS_SOURCE_ASSOCIATION_MAX}
                    />
                    <LimitOverrideField
                      name='access_source_switch_cooldown_minutes'
                      label={t('IP switch wait')}
                      description={t('0 = no wait')}
                      effectiveValue={
                        data.effective.access_source_switch_cooldown_minutes
                      }
                      source={
                        data.effective.access_source_switch_cooldown_source
                      }
                      max={ACCESS_SOURCE_SWITCH_COOLDOWN_MINUTES_MAX}
                    />
                  </div>
                  <p className='text-muted-foreground text-xs'>
                    {data.global.access_source_limit_enabled
                      ? t(
                          'Association period: {{hours}} hours · Max accounts per IP: {{count}}',
                          {
                            hours:
                              data.global
                                .access_source_association_window_hours,
                            count:
                              data.global.access_source_max_users_per_ip === 0
                                ? t('Unlimited')
                                : data.global.access_source_max_users_per_ip,
                          }
                        )
                      : t('Global access source limiting is disabled')}
                  </p>
                  <p className='text-muted-foreground text-xs'>
                    {t(
                      'IP switch wait only controls switching; association limits are checked separately'
                    )}
                  </p>
                </SideDrawerSection>

                <SideDrawerSection>
                  <SideDrawerSectionHeader
                    title={t('Access Source Records')}
                    description={t(
                      'View the current source, active associations, and recently rejected sources'
                    )}
                  />

                  <dl className='bg-border grid gap-px overflow-hidden rounded-lg border sm:grid-cols-3'>
                    <div className='bg-background p-3'>
                      <dt className='text-muted-foreground text-xs'>
                        {t('Current source IP')}
                      </dt>
                      <dd className='mt-1 font-mono text-sm font-medium'>
                        {state?.current_ip || '—'}
                      </dd>
                    </div>
                    <div className='bg-background p-3'>
                      <dt className='text-muted-foreground text-xs'>
                        {t('Associated IPs')}
                      </dt>
                      <dd className='mt-1 text-sm font-medium tabular-nums'>
                        {state?.associated_count ?? 0} /{' '}
                        {data.effective.access_source_max_ips || t('Unlimited')}
                      </dd>
                    </div>
                    <div className='bg-background p-3'>
                      <dt className='text-muted-foreground text-xs'>
                        {t('Switch wait remaining')}
                      </dt>
                      <dd className='mt-1 text-sm font-medium tabular-nums'>
                        {t('{{seconds}} seconds', {
                          seconds: state?.cooldown_remaining_seconds ?? 0,
                        })}
                      </dd>
                    </div>
                  </dl>

                  <div className='overflow-hidden rounded-lg border'>
                    <Table>
                      <TableHeader>
                        <TableRow>
                          <TableHead>{t('Source IP')}</TableHead>
                          <TableHead>{t('Status')}</TableHead>
                          <TableHead>{t('Last active')}</TableHead>
                          <TableHead className='text-right'>
                            {t('Action')}
                          </TableHead>
                        </TableRow>
                      </TableHeader>
                      <TableBody>
                        {associations.length === 0 ? (
                          <TableRow>
                            <TableCell
                              colSpan={4}
                              className='text-muted-foreground text-center'
                            >
                              {t('No active source associations')}
                            </TableCell>
                          </TableRow>
                        ) : (
                          associations.map((association) => (
                            <TableRow key={association.ip}>
                              <TableCell className='font-mono'>
                                {association.ip}
                              </TableCell>
                              <TableCell>
                                <Badge
                                  variant={
                                    association.is_current
                                      ? 'default'
                                      : 'secondary'
                                  }
                                >
                                  {association.is_current
                                    ? t('Current source')
                                    : t('Associated')}
                                </Badge>
                              </TableCell>
                              <TableCell>
                                {formatTimestamp(association.last_seen_at)}
                              </TableCell>
                              <TableCell className='text-right'>
                                {association.is_current ? (
                                  '—'
                                ) : (
                                  <Button
                                    type='button'
                                    size='sm'
                                    variant='outline'
                                    onClick={() =>
                                      setPendingAction({
                                        type: 'remove',
                                        ip: association.ip,
                                      })
                                    }
                                  >
                                    {t('Remove association')}
                                  </Button>
                                )}
                              </TableCell>
                            </TableRow>
                          ))
                        )}
                      </TableBody>
                    </Table>
                  </div>

                  {pending && (
                    <div className='flex flex-col gap-3 rounded-lg border p-3 sm:flex-row sm:items-center sm:justify-between'>
                      <div>
                        <div className='flex items-center gap-2'>
                          <span className='font-mono text-sm'>
                            {pending.ip}
                          </span>
                          <Badge variant='destructive'>{t('Rejected')}</Badge>
                        </div>
                        <p className='text-muted-foreground mt-1 text-xs'>
                          {pending.reason === 'switch_cooldown' &&
                          (state?.cooldown_remaining_seconds ?? 0) === 0
                            ? t(
                                'IP switch wait has ended; retrying will be checked against association limits'
                              )
                            : rejectionReason(t, pending.reason)}
                        </p>
                      </div>
                      <Button
                        type='button'
                        variant='outline'
                        onClick={() =>
                          setPendingAction({
                            type: 'allow',
                            rejection: pending,
                          })
                        }
                      >
                        {pending.reason === 'switch_cooldown'
                          ? t('Allow switch')
                          : t('Allow use')}
                      </Button>
                    </div>
                  )}

                  <div>
                    <div className='mb-2 flex flex-wrap items-center justify-between gap-2'>
                      <h4 className='text-sm font-medium'>
                        {t('Recent rejections')}
                      </h4>
                      <Badge variant='secondary'>
                        {t('Up to 10 records in 24 hours')}
                      </Badge>
                    </div>
                    {rejections.length === 0 ? (
                      <p className='text-muted-foreground text-sm'>
                        {t('No recent rejected sources')}
                      </p>
                    ) : (
                      <ul className='divide-y rounded-lg border'>
                        {rejections.map((rejection) => (
                          <li
                            key={rejection.event_id}
                            className='flex flex-wrap items-center justify-between gap-2 px-3 py-2'
                          >
                            <span className='font-mono text-sm'>
                              {rejection.ip}
                            </span>
                            <span className='text-muted-foreground text-xs'>
                              {rejectionReason(t, rejection.reason)} ·{' '}
                              {formatTimestamp(rejection.occurred_at)}
                            </span>
                          </li>
                        ))}
                      </ul>
                    )}
                  </div>
                </SideDrawerSection>
              </form>
            </Form>
          )}

          {data && (
            <SheetFooter className={sideDrawerFooterClassName()}>
              <SheetClose render={<Button variant='outline' />}>
                {t('Close')}
              </SheetClose>
              <Button
                type='submit'
                form='user-limit-settings-form'
                disabled={saveMutation.isPending}
              >
                {saveMutation.isPending ? t('Saving...') : t('Save changes')}
              </Button>
            </SheetFooter>
          )}
        </SheetContent>
      </Sheet>

      <ConfirmDialog
        open={pendingAction !== null}
        onOpenChange={(open) => !open && setPendingAction(null)}
        title={
          pendingAction?.type === 'remove'
            ? t('Remove source association')
            : t('Allow rejected source')
        }
        desc={
          pendingAction?.type === 'remove'
            ? t('Remove the association with {{ip}}?', {
                ip: pendingAction.ip,
              })
            : t(
                'Allow {{ip}} as the current source? The normal association expires after {{hours}} hours without activity and is not a permanent whitelist.',
                {
                  ip: pendingAction?.rejection.ip ?? '',
                  hours:
                    data?.global.access_source_association_window_hours ?? 0,
                }
              )
        }
        confirmText={
          pendingAction?.type === 'remove'
            ? t('Remove association')
            : t('Allow source')
        }
        destructive={pendingAction?.type === 'remove'}
        isLoading={sourceMutation.isPending}
        handleConfirm={() => {
          if (pendingAction) sourceMutation.mutate(pendingAction)
        }}
      />
    </>
  )
}
