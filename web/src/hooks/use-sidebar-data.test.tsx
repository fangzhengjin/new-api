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
import assert from 'node:assert/strict'

import { act, renderHook } from '@testing-library/react'
import { afterEach, describe, test } from 'vitest'

import { useSystemConfigStore } from '@/stores/system-config-store'

import { useSidebarData } from './use-sidebar-data'

describe('company quota management navigation', () => {
  afterEach(() => {
    useSystemConfigStore
      .getState()
      .setConfig({ companyQuotaModeEnabled: false })
  })

  test('switches quota-source menus with company mode', () => {
    useSystemConfigStore
      .getState()
      .setConfig({ companyQuotaModeEnabled: false })
    const { result } = renderHook(() => useSidebarData())
    const adminUrls = () =>
      result.current.navGroups
        .find((group) => group.id === 'admin')
        ?.items.flatMap((item) => ('url' in item ? [item.url] : [])) ?? []

    assert.equal(adminUrls().includes('/quota-management'), false)
    assert.equal(adminUrls().includes('/redemption-codes'), true)
    assert.equal(adminUrls().includes('/subscriptions'), true)

    act(() => {
      useSystemConfigStore
        .getState()
        .setConfig({ companyQuotaModeEnabled: true })
    })

    assert.equal(adminUrls().includes('/quota-management'), true)
    assert.equal(adminUrls().includes('/redemption-codes'), false)
    assert.equal(adminUrls().includes('/subscriptions'), false)
  })
})
