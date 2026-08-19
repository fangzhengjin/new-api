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
export const DEFAULT_OVERVIEW_PANEL_ORDER = [
  'api-info',
  'announcements',
  'faq',
  'uptime-kuma',
] as const

export type OverviewPanelId = (typeof DEFAULT_OVERVIEW_PANEL_ORDER)[number]

export type OverviewPanelSpan = 1 | 2 | 3

export type OverviewPanelLayoutItem = {
  id: OverviewPanelId
  span: OverviewPanelSpan
}

export const DEFAULT_OVERVIEW_PANEL_LAYOUT: readonly OverviewPanelLayoutItem[] =
  DEFAULT_OVERVIEW_PANEL_ORDER.map((id) => ({ id, span: 1 }))

const overviewPanelIds = new Set<string>(DEFAULT_OVERVIEW_PANEL_ORDER)

function createDefaultOverviewPanelLayout(): OverviewPanelLayoutItem[] {
  return DEFAULT_OVERVIEW_PANEL_LAYOUT.map((item) => ({ ...item }))
}

/**
 * Normalizes persisted panel layout while preserving legacy order arrays and
 * newly introduced panels.
 */
export function normalizeOverviewPanelLayout(
  value: unknown
): OverviewPanelLayoutItem[] {
  let parsed = value
  if (typeof value === 'string') {
    try {
      parsed = JSON.parse(value)
    } catch {
      return createDefaultOverviewPanelLayout()
    }
  }

  if (!Array.isArray(parsed)) return createDefaultOverviewPanelLayout()

  const seen = new Set<OverviewPanelId>()
  const layout: OverviewPanelLayoutItem[] = []
  for (const item of parsed) {
    const id = typeof item === 'string' ? item : item?.id
    if (
      typeof id === 'string' &&
      overviewPanelIds.has(id) &&
      !seen.has(id as OverviewPanelId)
    ) {
      const panelId = id as OverviewPanelId
      const span =
        typeof item === 'object' &&
        item !== null &&
        (item.span === 1 || item.span === 2 || item.span === 3)
          ? item.span
          : 1
      seen.add(panelId)
      layout.push({ id: panelId, span })
    }
  }

  for (const panelId of DEFAULT_OVERVIEW_PANEL_ORDER) {
    if (!seen.has(panelId)) layout.push({ id: panelId, span: 1 })
  }
  return layout
}
