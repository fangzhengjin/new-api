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
import { Wrench01Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { StatusBadge } from '@/components/status-badge'
import { Badge } from '@/components/ui/badge'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { formatLogQuota } from '@/lib/format'
import { cn } from '@/lib/utils'

import { LOG_TYPE_ENUM } from '../constants'
import { hasToolSurcharge } from '../lib/format'
import type { LogOtherData } from '../types'

interface LogCostDisplayProps {
  logType: number
  quota: number
  other: LogOtherData | null
}

const costToneConfig = {
  metered: {
    badge: 'border-border/80 bg-muted/60',
    variant: 'neutral',
  },
  subscription: {
    badge: 'border-success/35 bg-success/10',
    variant: 'success',
  },
  error: {
    badge: 'border-destructive/35 bg-destructive/10',
    variant: 'danger',
  },
} as const

type CostTone = keyof typeof costToneConfig

function splitQuotaDisplay(value: string): { prefix: string; amount: string } {
  const match = value.match(/^([^0-9+\-.,\s]+)(.+)$/)
  if (!match) return { prefix: '', amount: value }
  return { prefix: match[1], amount: match[2] }
}

function ToolSurchargeMarker() {
  const { t } = useTranslation()
  const label = t('Includes tool-call surcharge')

  return (
    <Tooltip>
      <TooltipTrigger
        render={
          <Badge
            variant='warning'
            className='h-5 min-w-5 cursor-help gap-0 rounded-full px-1'
            role='img'
            aria-label={label}
            tabIndex={0}
            data-tool-surcharge-indicator='true'
          >
            <HugeiconsIcon
              icon={Wrench01Icon}
              strokeWidth={2}
              aria-hidden='true'
            />
            <span
              className='text-[9px] leading-none font-bold'
              aria-hidden='true'
            >
              +
            </span>
          </Badge>
        }
      />
      <TooltipContent>{label}</TooltipContent>
    </Tooltip>
  )
}

function QuotaBadge(props: { label?: string; tone: CostTone; value: string }) {
  const [tooltipOpen, setTooltipOpen] = useState(false)
  const quotaDisplay = splitQuotaDisplay(props.value)
  const toneConfig = costToneConfig[props.tone]
  const badge = (
    <StatusBadge
      variant={toneConfig.variant}
      size='lg'
      showDot
      copyable={false}
      className={cn(
        'min-w-24 justify-start rounded-md border [font-family:var(--font-body)] text-foreground tabular-nums focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50',
        toneConfig.badge
      )}
      data-cost-tone={props.tone}
      aria-label={props.label ? `${props.label}: ${props.value}` : undefined}
      tabIndex={props.label ? 0 : undefined}
      onClick={props.label ? () => setTooltipOpen(true) : undefined}
    >
      <span className='inline-flex items-baseline gap-1'>
        {quotaDisplay.prefix ? <span>{quotaDisplay.prefix}</span> : null}
        <span>{quotaDisplay.amount}</span>
      </span>
    </StatusBadge>
  )

  if (!props.label) return badge

  return (
    <Tooltip open={tooltipOpen} onOpenChange={setTooltipOpen}>
      <TooltipTrigger closeOnClick={false} render={badge} />
      <TooltipContent>{props.label}</TooltipContent>
    </Tooltip>
  )
}

export function LogCostDisplay(props: LogCostDisplayProps) {
  const { t } = useTranslation()
  const isSubscription = props.other?.billing_source === 'subscription'
  const showToolSurcharge = hasToolSurcharge(props.other)
  let tone: CostTone = 'metered'
  let label: string | undefined

  if (props.logType === LOG_TYPE_ENUM.ERROR) {
    tone = 'error'
    label = t('Request failed, no fee was charged')
  } else if (isSubscription) {
    tone = 'subscription'
    label = t('Deducted by subscription')
  }

  const formattedQuota = formatLogQuota(props.quota)
  const quotaBadge = (
    <QuotaBadge label={label} tone={tone} value={formattedQuota} />
  )

  if (!label && !showToolSurcharge) {
    return <div className='flex flex-col gap-0.5'>{quotaBadge}</div>
  }

  return (
    <TooltipProvider>
      <div className='inline-flex items-center gap-1'>
        {quotaBadge}
        {showToolSurcharge ? <ToolSurchargeMarker /> : null}
      </div>
    </TooltipProvider>
  )
}
