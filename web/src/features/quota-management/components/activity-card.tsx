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
import { ArrowRight01Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import type { TFunction } from 'i18next'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

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
  Item,
  ItemActions,
  ItemContent,
  ItemDescription,
  ItemFooter,
  ItemGroup,
  ItemTitle,
} from '@/components/ui/item'

import { listQuotaActivities } from '../api'
import type { QuotaActivity } from '../types'
import { formatDateTime, formatQuota, queryKeys } from '../utils'
import { PageError, PageLoading } from './shared'

type ActivityIdentityData = {
  id?: number
  username?: string
  display_name?: string
}

type ActivityPresentation = {
  title: string
  subject?: ActivityIdentityData
  subjectLabel?: string
  subjectContext?: string
  summary?: string
  metric?: { label: string; value: string; negative?: boolean }
  facts: { label: string; value: string }[]
  link?: { type: 'plan' | 'cycle'; id: number }
}

function paramText(params: Record<string, unknown>, key: string) {
  const value = params[key]
  if (typeof value === 'string') return value.trim()
  if (typeof value === 'number' && Number.isFinite(value)) return String(value)
  return ''
}

function paramID(params: Record<string, unknown>, key: string) {
  const value = Number(paramText(params, key))
  return Number.isSafeInteger(value) && value > 0 ? value : undefined
}

function paramQuota(
  params: Record<string, unknown>,
  key: string,
  signed = false
) {
  const value = paramText(params, key)
  if (!/^-?\d+$/.test(value)) return ''
  return formatQuota(value, signed)
}

function netChangeQuota(params: Record<string, unknown>) {
  const recorded = paramText(params, 'net_change_quota')
  if (/^-?\d+$/.test(recorded)) return recorded
  const issued = paramText(params, 'issued_quota')
  const reclaimed = paramText(params, 'reclaimed_quota')
  if (!/^\d+$/.test(issued) || !/^\d+$/.test(reclaimed)) return ''
  return (BigInt(issued) - BigInt(reclaimed)).toString()
}

function identityPrimary(identity: ActivityIdentityData, t: TFunction) {
  return (
    identity.display_name?.trim() ||
    identity.username?.trim() ||
    (identity.id ? t('User {{id}}', { id: identity.id }) : t('System'))
  )
}

function ActivityIdentity(props: {
  identity: ActivityIdentityData
  label?: string
}) {
  const { t } = useTranslation()
  const displayName = props.identity.display_name?.trim()
  const username = props.identity.username?.trim()
  const secondary = displayName && username !== displayName ? username : ''
  return (
    <div className='min-w-0 text-left'>
      {props.label && (
        <div className='text-muted-foreground mb-1 text-xs'>{props.label}</div>
      )}
      <div className='truncate font-medium'>
        {identityPrimary(props.identity, t)}
      </div>
      {secondary && (
        <div className='text-muted-foreground truncate text-xs'>
          {secondary}
        </div>
      )}
    </div>
  )
}

function describeActivity(
  activity: QuotaActivity,
  t: TFunction
): ActivityPresentation {
  const params = activity.params
  const planID = paramID(params, 'plan_id')
  const cycleID = paramID(params, 'cycle_id')
  const reason = paramText(params, 'reason')
  const affected = paramText(params, 'affected_users')
  const issued = paramQuota(params, 'issued_quota')
  const reclaimed = paramQuota(params, 'reclaimed_quota')

  if (activity.action === 'user.quota_adjustment_plan') {
    const rawAdjustment = paramText(params, 'adjustment_quota')
    const adjustment = paramQuota(params, 'adjustment_quota', true)
    return {
      title: t('User quota adjusted'),
      subject: activity.target,
      subjectLabel: t('Adjustment target'),
      subjectContext: adjustment ? undefined : t('User quota adjusted'),
      summary: reason,
      metric: adjustment
        ? {
            label: rawAdjustment.startsWith('-')
              ? t('Decrease')
              : t('Increase'),
            value: adjustment,
            negative: rawAdjustment.startsWith('-'),
          }
        : undefined,
      facts: [
        ...(adjustment
          ? [{ label: t('Adjustment amount'), value: adjustment }]
          : []),
        ...(reason ? [{ label: t('Reason'), value: reason }] : []),
      ],
      link: planID ? { type: 'plan', id: planID } : undefined,
    }
  }

  if (activity.action === 'quota.plan.execute') {
    const netRaw = netChangeQuota(params)
    const net = netRaw ? formatQuota(netRaw, true) : ''
    return {
      title: planID
        ? t('Plan #{{id}} executed', { id: planID })
        : t('Plan executed'),
      summary:
        issued && reclaimed && affected
          ? t('Issued {{issued}} · Reclaimed {{reclaimed}} · {{count}} users', {
              issued,
              reclaimed,
              count: affected,
            })
          : undefined,
      metric: net
        ? {
            label: t('Net change'),
            value: net,
            negative: netRaw.startsWith('-'),
          }
        : undefined,
      facts: [
        ...(issued ? [{ label: t('Issued in this plan'), value: issued }] : []),
        ...(reclaimed
          ? [{ label: t('Reclaimed in this plan'), value: reclaimed }]
          : []),
        ...(net ? [{ label: t('Net change'), value: net }] : []),
        ...(affected ? [{ label: t('Affected users'), value: affected }] : []),
      ],
      link: planID ? { type: 'plan', id: planID } : undefined,
    }
  }

  const simple: Record<
    string,
    { title: string; link?: ActivityPresentation['link'] }
  > = {
    'quota.cycle.create': {
      title: cycleID
        ? t('Cycle #{{id}} created', { id: cycleID })
        : t('Cycle created'),
      link: cycleID ? { type: 'cycle', id: cycleID } : undefined,
    },
    'quota.cycle.update': {
      title: cycleID
        ? t('Cycle #{{id}} settings updated', { id: cycleID })
        : t('Cycle settings updated'),
      link: cycleID ? { type: 'cycle', id: cycleID } : undefined,
    },
    'quota.cycle.close': {
      title: cycleID
        ? t('Cycle #{{id}} closed', { id: cycleID })
        : t('Cycle closed'),
      link: cycleID ? { type: 'cycle', id: cycleID } : undefined,
    },
    'quota.plan.generate': {
      title: planID
        ? t('Plan #{{id}} generated', { id: planID })
        : t('Plan generated'),
      link: planID ? { type: 'plan', id: planID } : undefined,
    },
    'quota.plan.cancel': {
      title: planID
        ? t('Plan #{{id}} cancelled', { id: planID })
        : t('Plan cancelled'),
      link: planID ? { type: 'plan', id: planID } : undefined,
    },
    'quota.plan.regenerate': {
      title: planID
        ? t('Plan #{{id}} recalculated', { id: planID })
        : t('Plan recalculated'),
      link: planID ? { type: 'plan', id: planID } : undefined,
    },
    'quota.plan.notifications_retry': {
      title: planID
        ? t('Plan #{{id}} notifications retried', { id: planID })
        : t('Notifications retried'),
      link: planID ? { type: 'plan', id: planID } : undefined,
    },
  }

  const base = simple[activity.action]
  if (base) {
    const sourcePlanID = paramID(params, 'source_plan_id')
    const logsSent = paramText(params, 'logs_sent')
    const emailsSent = paramText(params, 'emails_sent')
    return {
      ...base,
      facts: [
        ...(sourcePlanID
          ? [{ label: t('Source plan'), value: `#${sourcePlanID}` }]
          : []),
        ...(logsSent
          ? [{ label: t('In-app notifications sent'), value: logsSent }]
          : []),
        ...(emailsSent ? [{ label: t('Emails sent'), value: emailsSent }] : []),
        ...(reason ? [{ label: t('Reason'), value: reason }] : []),
      ],
    }
  }

  if (
    activity.action === 'quota.temporary_request.approve' ||
    activity.action === 'quota.temporary_request.reject'
  ) {
    const approved = paramQuota(params, 'approved_quota')
    const approvedAction = activity.action === 'quota.temporary_request.approve'
    const requestID = paramID(params, 'request_id')
    let subjectContext: string | undefined = t('Temporary quota rejected')
    if (approvedAction) {
      subjectContext = approved ? undefined : t('Temporary quota approved')
    }
    return {
      title: approvedAction
        ? t('Temporary quota approved')
        : t('Temporary quota rejected'),
      subject: activity.target,
      subjectLabel: t('Requesting user'),
      subjectContext,
      summary: reason,
      metric: approved
        ? { label: t('Approved quota'), value: approved }
        : undefined,
      facts: [
        ...(requestID
          ? [{ label: t('Request ID'), value: `#${requestID}` }]
          : []),
        ...(approved ? [{ label: t('Approved quota'), value: approved }] : []),
        ...(reason ? [{ label: t('Review reason'), value: reason }] : []),
      ],
      link: planID ? { type: 'plan', id: planID } : undefined,
    }
  }

  if (activity.action === 'user.quota_whitelist') {
    const enabled =
      typeof params.enabled === 'boolean' ? params.enabled : undefined
    return {
      title: t('User management scope changed'),
      subject: activity.target,
      subjectLabel: t('User'),
      subjectContext:
        enabled === undefined ? t('User management scope changed') : undefined,
      metric:
        enabled === undefined
          ? undefined
          : {
              label: t('Quota management'),
              value: enabled
                ? t('Excluded from quota management')
                : t('Included in quota management'),
            },
      facts: [],
    }
  }

  return {
    title: t('Quota management action'),
    facts: [],
  }
}

export function ActivityCard() {
  const { t } = useTranslation()
  const [selected, setSelected] = useState<QuotaActivity | null>(null)
  const query = useQuery({
    queryKey: queryKeys.activities,
    queryFn: listQuotaActivities,
  })
  const selectedPresentation = selected
    ? describeActivity(selected, t)
    : undefined

  return (
    <>
      <Card>
        <CardHeader>
          <CardTitle>{t('Recent activity')}</CardTitle>
          <CardDescription>
            {t('The latest quota allocation and management actions')}
          </CardDescription>
        </CardHeader>
        <CardContent>
          {query.isPending && <PageLoading />}
          {query.isError && (
            <PageError
              title={t('Failed to load recent activity')}
              error={query.error}
            />
          )}
          {query.data?.length === 0 && (
            <p className='text-muted-foreground text-sm'>
              {t('No activity yet')}
            </p>
          )}
          {query.data && query.data.length > 0 && (
            <ItemGroup className='gap-0 overflow-hidden rounded-xl border'>
              {query.data.slice(0, 8).map((activity, index) => {
                const presentation = describeActivity(activity, t)
                const operator = {
                  id: activity.operator_id,
                  username: activity.operator,
                  display_name: activity.operator_display_name,
                }
                return (
                  <div key={activity.id} role='listitem'>
                    <Item
                      render={<button type='button' />}
                      className='hover:bg-muted/50 focus-visible:bg-muted/50 cursor-pointer rounded-none border-0 px-4 py-3 text-left'
                      onClick={() => setSelected(activity)}
                      aria-label={t('View activity details: {{activity}}', {
                        activity: presentation.subject
                          ? identityPrimary(presentation.subject, t)
                          : presentation.title,
                      })}
                    >
                      <ItemContent className='min-w-0'>
                        {presentation.subject ? (
                          <div className='min-w-0'>
                            {presentation.subjectContext && (
                              <ItemDescription>
                                {presentation.subjectContext}
                              </ItemDescription>
                            )}
                            <ActivityIdentity identity={presentation.subject} />
                          </div>
                        ) : (
                          <ItemTitle>{presentation.title}</ItemTitle>
                        )}
                        {presentation.summary && (
                          <ItemDescription>
                            {presentation.summary}
                          </ItemDescription>
                        )}
                      </ItemContent>
                      {presentation.metric && (
                        <ItemActions className='ml-auto flex-col items-end gap-0 self-start text-right'>
                          <span className='text-muted-foreground text-xs'>
                            {presentation.metric.label}
                          </span>
                          <span
                            className={
                              presentation.metric.negative
                                ? 'text-destructive font-semibold tabular-nums'
                                : 'font-semibold text-emerald-700 tabular-nums dark:text-emerald-400'
                            }
                          >
                            {presentation.metric.value}
                          </span>
                        </ItemActions>
                      )}
                      <ItemFooter className='border-border/60 mt-1 border-t pt-2'>
                        <ActivityIdentity
                          identity={operator}
                          label={t('Operator')}
                        />
                        <div className='flex shrink-0 items-center gap-2 self-end'>
                          <time className='text-muted-foreground text-xs tabular-nums'>
                            {formatDateTime(activity.created_at)}
                          </time>
                          <HugeiconsIcon
                            aria-hidden='true'
                            className='text-muted-foreground size-4'
                            icon={ArrowRight01Icon}
                            strokeWidth={2}
                          />
                        </div>
                      </ItemFooter>
                    </Item>
                    {index < Math.min(query.data.length, 8) - 1 && (
                      <div className='bg-border h-px' />
                    )}
                  </div>
                )
              })}
            </ItemGroup>
          )}
        </CardContent>
      </Card>

      <Dialog
        open={selected !== null}
        onOpenChange={(open) => {
          if (!open) setSelected(null)
        }}
      >
        {selected && selectedPresentation && (
          <DialogContent className='sm:max-w-lg'>
            <DialogHeader>
              <DialogTitle>{selectedPresentation.title}</DialogTitle>
              <DialogDescription>
                {formatDateTime(selected.created_at)}
              </DialogDescription>
            </DialogHeader>

            <div
              className={
                selectedPresentation.subject
                  ? 'grid gap-3 sm:grid-cols-2'
                  : 'grid gap-3'
              }
            >
              {selectedPresentation.subject && (
                <div className='bg-muted/50 rounded-lg p-3'>
                  <ActivityIdentity
                    identity={selectedPresentation.subject}
                    label={selectedPresentation.subjectLabel}
                  />
                </div>
              )}
              <div className='bg-muted/50 rounded-lg p-3'>
                <ActivityIdentity
                  identity={{
                    id: selected.operator_id,
                    username: selected.operator,
                    display_name: selected.operator_display_name,
                  }}
                  label={t('Operator')}
                />
              </div>
            </div>

            {selectedPresentation.facts.length > 0 && (
              <dl className='divide-border divide-y rounded-lg border px-3'>
                {selectedPresentation.facts.map((fact) => (
                  <div
                    key={fact.label}
                    className='grid gap-1 py-2.5 sm:grid-cols-[9rem_1fr]'
                  >
                    <dt className='text-muted-foreground'>{fact.label}</dt>
                    <dd className='font-medium break-words tabular-nums'>
                      {fact.value}
                    </dd>
                  </div>
                ))}
              </dl>
            )}

            {!selectedPresentation.link &&
              selected.action === 'user.quota_adjustment_plan' && (
                <p className='text-muted-foreground text-sm'>
                  {t(
                    'Only the recorded audit information is available for this historical activity'
                  )}
                </p>
              )}

            {selectedPresentation.link && (
              <DialogFooter>
                <Button
                  render={
                    selectedPresentation.link.type === 'plan' ? (
                      <Link
                        to='/quota-management/plans/$planId'
                        params={{
                          planId: String(selectedPresentation.link.id),
                        }}
                      />
                    ) : (
                      <Link
                        to='/quota-management/cycles/$cycleId'
                        params={{
                          cycleId: String(selectedPresentation.link.id),
                        }}
                      />
                    )
                  }
                >
                  {selectedPresentation.link.type === 'plan'
                    ? t('View plan details')
                    : t('View cycle details')}
                  <HugeiconsIcon
                    icon={ArrowRight01Icon}
                    strokeWidth={2}
                    data-icon='inline-end'
                  />
                </Button>
              </DialogFooter>
            )}
          </DialogContent>
        )}
      </Dialog>
    </>
  )
}
