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
import { describe, expect, it, vi } from 'vitest'

import { isApiKeyUsable } from '../../api'
import {
  chatLinkRequiresBackendLaunch,
  chatPresetOpensInNewTab,
  getChatIframeSandbox,
  parseChatConfig,
  parseChatConfigEntries,
  postChatTheme,
  resolveChatUrl,
  serializeChatConfigEntry,
  shouldGroupChatEntries,
} from '../chat-links'

const usableKey = {
  id: 1,
  name: 'Canvas',
  key: 'masked',
  status: 1,
  remain_quota: 1,
  used_quota: 0,
  unlimited_quota: false,
  expired_time: -1,
  created_time: 0,
  accessed_time: 0,
  group: '',
  auto_groups: null,
  cross_group_retry: false,
  model_limits_enabled: false,
  model_limits: '',
  allow_ips: '',
}

describe('chat preset menu rules', () => {
  it('groups only when the visible entry count is greater than the threshold', () => {
    expect(shouldGroupChatEntries(3, 3)).toBe(false)
    expect(shouldGroupChatEntries(4, 3)).toBe(true)
    expect(shouldGroupChatEntries(1, 0)).toBe(true)
  })

  it('parses only standard entries and hides explicitly disabled entries', () => {
    const raw = [
      {
        name: 'Embedded',
        url: 'https://embedded.example',
        enabled: true,
        icon: 'Palette',
        sandbox: ['allow-scripts', 'allow-same-origin'],
      },
      {
        name: 'New tab',
        url: 'https://new-tab.example',
        enabled: true,
        open_mode: 'new_tab',
      },
      {
        name: 'Disabled',
        url: 'https://disabled.example',
        enabled: false,
      },
      {
        name: 'Invalid icon',
        url: 'https://invalid-icon.example',
        enabled: true,
        icon: 1,
      },
      {
        name: 'Unsafe icon',
        url: 'https://unsafe-icon.example',
        enabled: true,
        icon: '<Palette>',
      },
      {
        name: 'Invalid sandbox',
        url: 'https://invalid.example',
        enabled: true,
        sandbox: ['allow-navigation'],
      },
      { Legacy: 'https://legacy.example' },
    ]

    expect(parseChatConfigEntries(raw)).toEqual([
      {
        name: 'Embedded',
        url: 'https://embedded.example',
        enabled: true,
        icon: 'Palette',
        openMode: 'embedded',
        sandbox: ['allow-scripts', 'allow-same-origin'],
      },
      {
        name: 'New tab',
        url: 'https://new-tab.example',
        enabled: true,
        openMode: 'new_tab',
      },
      {
        name: 'Disabled',
        url: 'https://disabled.example',
        enabled: false,
        openMode: 'embedded',
      },
    ])
    expect(parseChatConfig(raw).map(({ id, name }) => ({ id, name }))).toEqual([
      { id: '0', name: 'Embedded' },
      { id: '1', name: 'New tab' },
    ])
  })

  it('serializes the standard object format', () => {
    expect(
      serializeChatConfigEntry({
        name: 'Canvas',
        url: 'https://canvas.example',
        enabled: false,
        icon: 'Palette',
        openMode: 'embedded',
        sandbox: ['allow-scripts', 'allow-same-origin'],
      })
    ).toEqual({
      name: 'Canvas',
      url: 'https://canvas.example',
      enabled: false,
      icon: 'Palette',
      open_mode: 'embedded',
      sandbox: ['allow-scripts', 'allow-same-origin'],
    })
  })

  it('derives backend launch and new-tab behavior from variables and open mode', () => {
    const embedded = parseChatConfig([
      {
        name: 'Canvas',
        url: 'https://canvas.example/#code={authCode}',
        enabled: true,
      },
    ])[0]
    const external = { ...embedded, openMode: 'new_tab' as const }

    expect(chatLinkRequiresBackendLaunch(embedded.url)).toBe(true)
    expect(chatPresetOpensInNewTab(embedded)).toBe(false)
    expect(chatPresetOpensInNewTab(external)).toBe(true)
    expect(chatLinkRequiresBackendLaunch('https://example.com')).toBe(false)
  })

  it('renders the resolved light or dark theme into chat URLs', () => {
    expect(
      resolveChatUrl({
        template: 'https://example.com/?theme={theme}',
        serverAddress: 'https://api.example.com',
        theme: 'dark',
      })
    ).toBe('https://example.com/?theme=dark')
  })

  it('posts theme changes to the selected iframe window', () => {
    const postMessage = vi.fn()

    postChatTheme({ postMessage } as unknown as Window, 'dark')

    expect(postMessage).toHaveBeenCalledWith(
      { type: 'theme', value: 'dark' },
      '*'
    )
  })

  it('grants origin storage only to a separately hosted embedded app', () => {
    const external = getChatIframeSandbox(
      'https://canvas.example/image',
      'https://gateway.example'
    )
    const sameOrigin = getChatIframeSandbox(
      'https://gateway.example/chat',
      'https://gateway.example'
    )

    expect(external).toContain('allow-same-origin')
    expect(sameOrigin).not.toContain('allow-same-origin')
  })

  it('uses an explicitly configured sandbox instead of the default permissions', () => {
    expect(
      getChatIframeSandbox(
        'https://gateway.example/chat',
        'https://gateway.example',
        ['allow-scripts', 'allow-same-origin']
      )
    ).toBe('allow-scripts allow-same-origin')
    expect(
      getChatIframeSandbox(
        'https://canvas.example/chat',
        'https://gateway.example',
        []
      )
    ).toBe('')
  })

  it('accepts only enabled, unexpired API keys with quota', () => {
    expect(isApiKeyUsable(usableKey, 100)).toBe(true)
    expect(isApiKeyUsable({ ...usableKey, status: 2 }, 100)).toBe(false)
    expect(isApiKeyUsable({ ...usableKey, expired_time: 99 }, 100)).toBe(false)
    expect(isApiKeyUsable({ ...usableKey, remain_quota: 0 }, 100)).toBe(false)
    expect(
      isApiKeyUsable(
        { ...usableKey, unlimited_quota: true, remain_quota: 0 },
        100
      )
    ).toBe(true)
  })
})
