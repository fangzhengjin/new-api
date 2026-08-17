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

const overviewPanelIds = new Set<string>(DEFAULT_OVERVIEW_PANEL_ORDER)

/**
 * Normalizes persisted panel order while preserving newly introduced panels.
 */
export function normalizeOverviewPanelOrder(value: unknown): OverviewPanelId[] {
  let parsed = value
  if (typeof value === 'string') {
    try {
      parsed = JSON.parse(value)
    } catch {
      return [...DEFAULT_OVERVIEW_PANEL_ORDER]
    }
  }

  if (!Array.isArray(parsed)) return [...DEFAULT_OVERVIEW_PANEL_ORDER]

  const seen = new Set<OverviewPanelId>()
  const order: OverviewPanelId[] = []
  for (const item of parsed) {
    if (
      typeof item === 'string' &&
      overviewPanelIds.has(item) &&
      !seen.has(item as OverviewPanelId)
    ) {
      const panelId = item as OverviewPanelId
      seen.add(panelId)
      order.push(panelId)
    }
  }

  for (const panelId of DEFAULT_OVERVIEW_PANEL_ORDER) {
    if (!seen.has(panelId)) order.push(panelId)
  }
  return order
}
