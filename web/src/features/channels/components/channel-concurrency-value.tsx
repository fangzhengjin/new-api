/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { useQuery } from '@tanstack/react-query'
import { createContext, useContext, useMemo, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'

import { getChannelConcurrency } from '../api'
import {
  channelsQueryKeys,
  formatChannelConcurrency,
  shouldFetchChannelConcurrency,
} from '../lib'

type ChannelConcurrencyCounts = Record<
  string,
  { active: number; waiting: number }
>

type ChannelConcurrencyContextValue = {
  available: boolean
  counts?: ChannelConcurrencyCounts
}

const ChannelConcurrencyContext = createContext<ChannelConcurrencyContextValue>(
  { available: false }
)

/**
 * Poll channel concurrency without causing the surrounding table to rebuild.
 * @param props - Visible channel IDs, column visibility, and table/card content.
 * @returns A narrow context provider consumed only by concurrency values.
 */
export function ChannelConcurrencyProvider(props: {
  channelIds: number[]
  enabled: boolean
  children: ReactNode
}) {
  const queryEnabled = shouldFetchChannelConcurrency(
    props.channelIds,
    props.enabled
  )
  const query = useQuery({
    queryKey: channelsQueryKeys.concurrency(props.channelIds),
    queryFn: () => getChannelConcurrency(props.channelIds),
    enabled: queryEnabled,
    refetchInterval: queryEnabled ? 3000 : false,
    refetchIntervalInBackground: false,
  })
  const value = useMemo<ChannelConcurrencyContextValue>(
    () => ({
      available:
        queryEnabled && !query.isError && query.data?.data?.available === true,
      counts: queryEnabled ? query.data?.data?.counts : undefined,
    }),
    [
      query.data?.data?.available,
      query.data?.data?.counts,
      query.isError,
      queryEnabled,
    ]
  )

  return (
    <ChannelConcurrencyContext.Provider value={value}>
      {props.children}
    </ChannelConcurrencyContext.Provider>
  )
}

interface ChannelConcurrencyValueProps {
  channelId: number
  limit: number
}

/**
 * Render active, limit, and optional waiting channel request counts.
 * @param props - Current Redis snapshot and configured channel limit.
 * @returns A compact concurrency value with an accessible waiting tooltip.
 */
export function ChannelConcurrencyValue(props: ChannelConcurrencyValueProps) {
  const { t } = useTranslation()
  const snapshot = useContext(ChannelConcurrencyContext)
  const counts = snapshot.counts?.[String(props.channelId)]
  const waiting = snapshot.available ? (counts?.waiting ?? 0) : 0

  return (
    <span className='inline-flex items-center gap-1 font-mono text-sm tabular-nums'>
      <span>
        {formatChannelConcurrency(
          counts?.active,
          props.limit,
          snapshot.available
        )}
      </span>
      {waiting > 0 && (
        <TooltipProvider delay={100}>
          <Tooltip>
            <TooltipTrigger
              render={
                <span
                  className='cursor-help text-amber-600 dark:text-amber-400'
                  tabIndex={0}
                  aria-label={t('Waiting requests: {{count}}', {
                    count: waiting,
                  })}
                />
              }
            >
              · {waiting}
            </TooltipTrigger>
            <TooltipContent side='top'>
              {t('Waiting requests: {{count}}', { count: waiting })}
            </TooltipContent>
          </Tooltip>
        </TooltipProvider>
      )}
    </span>
  )
}
