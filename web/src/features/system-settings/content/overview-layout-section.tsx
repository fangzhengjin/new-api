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
import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import {
  DEFAULT_OVERVIEW_PANEL_LAYOUT,
  normalizeOverviewPanelLayout,
  type OverviewPanelId,
  type OverviewPanelSpan,
} from '@/features/dashboard/lib/overview-panels'

import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useUpdateOption } from '../hooks/use-update-option'
import { OverviewLayoutPreview } from './overview-layout-preview'

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
 * Configures the global order and grid span of overview content panels.
 */
export function OverviewLayoutSection(props: OverviewLayoutSectionProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const defaultLayout = useMemo(
    () => normalizeOverviewPanelLayout(props.defaultValue),
    [props.defaultValue]
  )
  const [panelLayout, setPanelLayout] = useState(defaultLayout)

  useEffect(() => {
    setPanelLayout(defaultLayout)
  }, [defaultLayout])

  const hasChanges = panelLayout.some(
    (item, index) =>
      item.id !== defaultLayout[index]?.id ||
      item.span !== defaultLayout[index]?.span
  )
  const usesDefaultLayout = panelLayout.every(
    (item, index) =>
      item.id === DEFAULT_OVERVIEW_PANEL_LAYOUT[index]?.id &&
      item.span === DEFAULT_OVERVIEW_PANEL_LAYOUT[index]?.span
  )
  const visibleLayout = panelLayout.filter(
    (item) => props.enabledPanels[item.id]
  )
  const panelNames = Object.fromEntries(
    Object.entries(overviewPanels).map(([panelId, panel]) => [
      panelId,
      t(panel.titleKey),
    ])
  ) as Record<OverviewPanelId, string>

  const movePanel = (
    panelId: OverviewPanelId,
    targetPanelId: OverviewPanelId
  ) => {
    setPanelLayout((current) => {
      const visible = current.filter((item) => props.enabledPanels[item.id])
      const sourceIndex = visible.findIndex((item) => item.id === panelId)
      const targetIndex = visible.findIndex((item) => item.id === targetPanelId)
      if (sourceIndex < 0 || targetIndex < 0 || sourceIndex === targetIndex) {
        return current
      }

      const nextVisible = [...visible]
      const [source] = nextVisible.splice(sourceIndex, 1)
      nextVisible.splice(targetIndex, 0, source)
      let visibleIndex = 0
      return current.map((item) =>
        props.enabledPanels[item.id] ? nextVisible[visibleIndex++] : item
      )
    })
  }

  const movePanelByOffset = (panelId: OverviewPanelId, offset: -1 | 1) => {
    const index = visibleLayout.findIndex((item) => item.id === panelId)
    const targetPanel = visibleLayout[index + offset]
    if (targetPanel) movePanel(panelId, targetPanel.id)
  }

  const changePanelSpan = (
    panelId: OverviewPanelId,
    span: OverviewPanelSpan
  ) => {
    setPanelLayout((current) =>
      current.map((item) => (item.id === panelId ? { ...item, span } : item))
    )
  }

  const handleSave = async () => {
    await updateOption.mutateAsync({
      key: 'console_setting.overview_panel_order',
      value: JSON.stringify(panelLayout),
    })
  }

  return (
    <SettingsSection title={t('Overview Layout')}>
      <SettingsPageFormActions
        onSave={handleSave}
        onReset={() =>
          setPanelLayout(
            DEFAULT_OVERVIEW_PANEL_LAYOUT.map((item) => ({ ...item }))
          )
        }
        isSaving={updateOption.isPending}
        isSaveDisabled={!hasChanges}
        isResetDisabled={usesDefaultLayout}
        resetLabel='Restore defaults'
      />

      <div className='flex max-w-3xl flex-col gap-1'>
        <h3 className='text-sm font-medium'>{t('Overview content order')}</h3>
        <p className='text-muted-foreground text-sm leading-relaxed'>
          {t(
            'Drag cards to reorder. Drag the right edge to change their width.'
          )}
        </p>
      </div>

      <OverviewLayoutPreview
        items={visibleLayout}
        panelNames={panelNames}
        onMove={movePanel}
        onMoveByOffset={movePanelByOffset}
        onSpanChange={changePanelSpan}
      />
    </SettingsSection>
  )
}
