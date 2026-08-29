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

import { useAuthStore } from '@/stores/auth-store'
import { useSystemConfigStore } from '@/stores/system-config-store'

import { useSidebarData } from './use-sidebar-data'

describe('cycle quota management navigation', () => {
  afterEach(() => {
    useAuthStore.getState().auth.setUser(null)
    useSystemConfigStore
      .getState()
      .setConfig({ cycleQuotaManagementEnabled: false })
  })

  test('adds quota management without hiding unrelated admin menus', () => {
    useSystemConfigStore
      .getState()
      .setConfig({ cycleQuotaManagementEnabled: false })
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
        .setConfig({ cycleQuotaManagementEnabled: true })
    })

    assert.equal(adminUrls().includes('/quota-management'), true)
    assert.equal(adminUrls().includes('/redemption-codes'), true)
    assert.equal(adminUrls().includes('/subscriptions'), true)
  })

  test('keeps temporary quota independent from the profile module', () => {
    useAuthStore.getState().auth.setUser({
      id: 1,
      username: 'eligible-user',
      role: 1,
      temporary_quota_request_eligible: true,
    })
    useSystemConfigStore
      .getState()
      .setConfig({ cycleQuotaManagementEnabled: true })
    const { result } = renderHook(() => useSidebarData())
    const item = result.current.navGroups
      .find((group) => group.id === 'personal')
      ?.items.find(
        (entry) => 'url' in entry && entry.url === '/temporary-quota'
      )

    assert.ok(item)
    assert.equal('configUrls' in item, false)
  })

  test('hides temporary quota until user eligibility is available and true', () => {
    useSystemConfigStore
      .getState()
      .setConfig({ cycleQuotaManagementEnabled: true })
    const { result } = renderHook(() => useSidebarData())
    const hasMenu = () =>
      result.current.navGroups.some((group) =>
        group.items.some(
          (item) => 'url' in item && item.url === '/temporary-quota'
        )
      )
    assert.equal(hasMenu(), false)
    act(() =>
      useAuthStore.getState().auth.setUser({
        id: 1,
        username: 'eligible-user',
        role: 1,
        temporary_quota_request_eligible: true,
      })
    )
    assert.equal(hasMenu(), true)
    act(() =>
      useAuthStore.getState().auth.setUser({
        id: 1,
        username: 'ineligible-user',
        role: 1,
        temporary_quota_request_eligible: false,
      })
    )
    assert.equal(hasMenu(), false)
  })
})
