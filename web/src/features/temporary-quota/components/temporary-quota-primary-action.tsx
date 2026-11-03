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
import { Plus } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'

import type { TemporaryQuotaOverview } from '../types'

export function TemporaryQuotaPrimaryAction(props: {
  overview?: TemporaryQuotaOverview
  onOpen: () => void
}) {
  const { t } = useTranslation()

  const canRequest = props.overview?.can_request === true
  const unavailableReason = props.overview?.unavailable_reason
    ? t(props.overview.unavailable_reason)
    : ''

  const button = (
    <Button
      size='sm'
      disabled={!canRequest}
      onClick={props.onOpen}
      aria-describedby={
        canRequest || !unavailableReason
          ? undefined
          : 'temporary-quota-unavailable'
      }
    >
      <Plus className='size-4' aria-hidden='true' />
      {t('Request temporary quota')}
    </Button>
  )

  if (canRequest || !unavailableReason) return button

  return (
    <Tooltip>
      <TooltipTrigger
        render={
          <span
            className='focus-visible:outline-ring inline-flex rounded-md focus-visible:outline-2 focus-visible:outline-offset-2'
            tabIndex={0}
            aria-label={`${t('Request temporary quota')}: ${unavailableReason}`}
          />
        }
      >
        {button}
      </TooltipTrigger>
      <TooltipContent id='temporary-quota-unavailable'>
        {unavailableReason}
      </TooltipContent>
    </Tooltip>
  )
}
