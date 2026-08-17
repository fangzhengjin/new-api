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
import { Link } from '@tanstack/react-router'
import { ChevronDown, ChevronUp } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  DEFAULT_OVERVIEW_PANEL_ORDER,
  normalizeOverviewPanelOrder,
  type OverviewPanelId,
} from '@/features/dashboard/lib/overview-panels'

import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useUpdateOption } from '../hooks/use-update-option'

const overviewPanels: Record<OverviewPanelId, { titleKey: string }> = {
  'api-info': {
    titleKey: 'API Info',
  },
  announcements: {
    titleKey: 'Announcements',
  },
  faq: {
    titleKey: 'FAQ',
  },
  'uptime-kuma': {
    titleKey: 'Uptime Kuma',
  },
}

type OverviewLayoutSectionProps = {
  defaultValue: string
  enabledPanels: Record<OverviewPanelId, boolean>
}

/**
 * Configures the global reading order of overview content panels.
 */
export function OverviewLayoutSection(props: OverviewLayoutSectionProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const defaultOrder = useMemo(
    () => normalizeOverviewPanelOrder(props.defaultValue),
    [props.defaultValue]
  )
  const [panelOrder, setPanelOrder] = useState(defaultOrder)

  useEffect(() => {
    setPanelOrder(defaultOrder)
  }, [defaultOrder])

  const hasChanges = panelOrder.some(
    (panelId, index) => panelId !== defaultOrder[index]
  )
  const usesDefaultOrder = panelOrder.every(
    (panelId, index) => panelId === DEFAULT_OVERVIEW_PANEL_ORDER[index]
  )

  const movePanel = (panelId: OverviewPanelId, offset: -1 | 1) => {
    setPanelOrder((current) => {
      const index = current.indexOf(panelId)
      const targetIndex = index + offset
      if (index < 0 || targetIndex < 0 || targetIndex >= current.length) {
        return current
      }

      const next = [...current]
      const targetPanel = next[targetIndex]
      next[targetIndex] = next[index]
      next[index] = targetPanel
      return next
    })
  }

  const handleSave = async () => {
    await updateOption.mutateAsync({
      key: 'console_setting.overview_panel_order',
      value: JSON.stringify(panelOrder),
    })
  }

  return (
    <SettingsSection title={t('Overview Layout')}>
      <SettingsPageFormActions
        onSave={handleSave}
        onReset={() => setPanelOrder([...DEFAULT_OVERVIEW_PANEL_ORDER])}
        isSaving={updateOption.isPending}
        isSaveDisabled={!hasChanges}
        isResetDisabled={usesDefaultOrder}
        resetLabel='Restore defaults'
      />

      <div className='flex max-w-3xl flex-col gap-1'>
        <h3 className='text-sm font-medium'>{t('Overview content order')}</h3>
        <p className='text-muted-foreground text-sm leading-relaxed'>
          {t(
            'Arrange overview content panels. Disabled panels keep their position and return there when enabled.'
          )}
        </p>
      </div>

      <ol className='overflow-hidden rounded-xl border'>
        {panelOrder.map((panelId, index) => {
          const panel = overviewPanels[panelId]
          const panelName = t(panel.titleKey)
          const enabled = props.enabledPanels[panelId]

          return (
            <li
              key={panelId}
              className='flex min-w-0 items-center gap-3 px-3 py-3 not-last:border-b sm:px-4'
            >
              <span className='text-muted-foreground w-5 shrink-0 text-center text-sm tabular-nums'>
                {index + 1}
              </span>
              <div className='flex min-w-0 flex-1 flex-wrap items-center gap-2'>
                <span className='truncate text-sm font-medium'>
                  {panelName}
                </span>
                <Badge variant={enabled ? 'secondary' : 'outline'}>
                  {t(enabled ? 'Enabled' : 'Disabled')}
                </Badge>
              </div>
              <div className='flex shrink-0 items-center gap-1'>
                <Button
                  variant='ghost'
                  size='sm'
                  render={
                    <Link
                      to='/system-settings/content/$section'
                      params={{ section: panelId }}
                    />
                  }
                >
                  {t('Configure')}
                </Button>
                <Button
                  type='button'
                  variant='outline'
                  size='icon-sm'
                  onClick={() => movePanel(panelId, -1)}
                  disabled={index === 0}
                  aria-label={t('Move {{name}} up', { name: panelName })}
                  title={t('Move {{name}} up', { name: panelName })}
                >
                  <ChevronUp aria-hidden='true' />
                </Button>
                <Button
                  type='button'
                  variant='outline'
                  size='icon-sm'
                  onClick={() => movePanel(panelId, 1)}
                  disabled={index === panelOrder.length - 1}
                  aria-label={t('Move {{name}} down', { name: panelName })}
                  title={t('Move {{name}} down', { name: panelName })}
                >
                  <ChevronDown aria-hidden='true' />
                </Button>
              </div>
            </li>
          )
        })}
      </ol>

      <p className='text-muted-foreground text-xs'>
        {t('Only enabled panels appear on the overview page.')}
      </p>
    </SettingsSection>
  )
}
