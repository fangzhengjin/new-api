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
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, test, vi } from 'vitest'

import { SettingsPageProvider } from '../../../components/settings-page-context'
import { ChannelAffinitySection } from '../index'

const { mutateAsync } = vi.hoisted(() => ({
  mutateAsync: vi.fn().mockResolvedValue(undefined),
}))

vi.mock('../../../hooks/use-update-option', () => ({
  useUpdateOption: () => ({ mutateAsync, isPending: false }),
}))

vi.mock('../api', () => ({
  getCacheStats: vi.fn().mockResolvedValue({
    success: true,
    data: {
      enabled: true,
      total: 0,
      unknown: 0,
      by_rule_name: {},
      cache_capacity: 100000,
      cache_algo: 'LRU',
    },
  }),
  clearAllCache: vi.fn(),
  clearRuleCache: vi.fn(),
}))

describe('Channel affinity TTL renewal setting', () => {
  beforeEach(() => {
    mutateAsync.mockClear()
  })

  test('saves fixed TTL mode independently from other affinity settings', async () => {
    const actionsContainer = document.createElement('div')
    document.body.appendChild(actionsContainer)

    render(
      <SettingsPageProvider actionsContainer={actionsContainer}>
        <ChannelAffinitySection
          defaultValues={{
            'channel_affinity_setting.enabled': true,
            'channel_affinity_setting.renew_ttl_on_success': true,
            'channel_affinity_setting.switch_on_success': true,
            'channel_affinity_setting.keep_on_channel_disabled': false,
            'channel_affinity_setting.max_entries': 100000,
            'channel_affinity_setting.default_ttl_seconds': 300,
            'channel_affinity_setting.rules': '[]',
          }}
        />
      </SettingsPageProvider>
    )

    fireEvent.click(
      screen.getByRole('switch', {
        name: 'Renew affinity TTL on success',
      })
    )
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))

    await waitFor(() => {
      expect(mutateAsync).toHaveBeenCalledWith({
        key: 'channel_affinity_setting.renew_ttl_on_success',
        value: 'false',
      })
    })
  })
})
