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
import { describe, expect, test } from 'vitest'

import {
  DEFAULT_OVERVIEW_PANEL_LAYOUT,
  normalizeOverviewPanelLayout,
} from '../overview-panels'

describe('overview panel layout', () => {
  test('keeps legacy order arrays and appends missing panels', () => {
    expect(normalizeOverviewPanelLayout('["faq","unknown","faq"]')).toEqual([
      { id: 'faq', span: 1 },
      { id: 'api-info', span: 1 },
      { id: 'announcements', span: 1 },
      { id: 'uptime-kuma', span: 1 },
    ])
  })

  test('keeps valid spans and normalizes invalid spans', () => {
    expect(
      normalizeOverviewPanelLayout(
        '[{"id":"announcements","span":2},{"id":"faq","span":4}]'
      )
    ).toEqual([
      { id: 'announcements', span: 2 },
      { id: 'faq', span: 1 },
      { id: 'api-info', span: 1 },
      { id: 'uptime-kuma', span: 1 },
    ])
  })

  test('falls back when the stored value is invalid', () => {
    expect(normalizeOverviewPanelLayout('{invalid')).toEqual(
      DEFAULT_OVERVIEW_PANEL_LAYOUT
    )
  })
})
