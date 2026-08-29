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
import { API_KEY_STATUS } from '@/features/keys/constants'

export type ChatLinkType = 'web' | 'custom-protocol' | 'fluent'
export type ChatOpenMode = 'embedded' | 'new_tab'
export type ChatTheme = 'light' | 'dark'

export const CHAT_SANDBOX_PERMISSIONS = [
  'allow-downloads',
  'allow-forms',
  'allow-modals',
  'allow-popups',
  'allow-popups-to-escape-sandbox',
  'allow-presentation',
  'allow-scripts',
  'allow-same-origin',
] as const

export type ChatSandboxPermission = (typeof CHAT_SANDBOX_PERMISSIONS)[number]

export type ChatPreset = {
  id: string
  name: string
  url: string
  type: ChatLinkType
  icon?: string
  openMode?: ChatOpenMode
  sandbox?: ChatSandboxPermission[]
}

export type ChatConfigEntry = {
  name: string
  url: string
  enabled: boolean
  icon?: string
  openMode?: ChatOpenMode
  sandbox?: ChatSandboxPermission[]
}

export type RawChatConfig =
  | string
  | Record<string, unknown>
  | Array<Record<string, unknown>>
  | null
  | undefined

export type ResolveChatUrlParams = {
  template: string
  apiKey?: string
  serverAddress: string
  theme: ChatTheme
}

export type ActiveApiKey = {
  key: string
  status: number
}

const HTTP_REGEX = /^https?:\/\//i
const CHAT_ICON_PATTERN = /^[A-Za-z0-9][A-Za-z0-9 _-]{0,63}$/
const CHAT_IFRAME_SANDBOX = CHAT_SANDBOX_PERMISSIONS.filter(
  (permission) => permission !== 'allow-same-origin'
).join(' ')
const CHAT_SANDBOX_PERMISSION_SET = new Set<string>(CHAT_SANDBOX_PERMISSIONS)

function toBase64(value: string) {
  if (typeof window !== 'undefined' && typeof window.btoa === 'function') {
    return window.btoa(value)
  }

  type BufferConstructorLike = {
    from(data: string, encoding: string): { toString(encoding: string): string }
  }

  const globalObj =
    typeof globalThis !== 'undefined'
      ? (globalThis as Record<string, unknown>)
      : undefined
  const bufferCtor = globalObj?.Buffer

  if (
    typeof bufferCtor === 'function' &&
    typeof (bufferCtor as unknown as BufferConstructorLike).from === 'function'
  ) {
    return (bufferCtor as unknown as BufferConstructorLike)
      .from(value, 'utf-8')
      .toString('base64')
  }

  return ''
}

export function detectChatLinkType(url: string): ChatLinkType {
  if (HTTP_REGEX.test(url)) {
    return 'web'
  }
  if (url.toLowerCase().startsWith('fluent')) {
    return 'fluent'
  }
  return 'custom-protocol'
}

export function chatLinkRequiresApiKey(url: string): boolean {
  return (
    url.includes('{key}') ||
    url.includes('{cherryConfig}') ||
    url.includes('{aionuiConfig}') ||
    url.includes('{deepchatConfig}') ||
    url.includes('{aqbotConfig}')
  )
}

export function chatLinkRequiresBackendLaunch(url: string): boolean {
  return (
    url.includes('{authCode}') ||
    url.includes('{textModels}') ||
    url.includes('{imageModels}') ||
    url.includes('{videoModels}')
  )
}

export function chatPresetOpensInNewTab(preset: ChatPreset): boolean {
  return preset.type !== 'web' || preset.openMode === 'new_tab'
}

export function getChatIframeSandbox(
  url: string,
  currentOrigin: string,
  configuredSandbox?: readonly ChatSandboxPermission[]
): string {
  if (configuredSandbox !== undefined) return configuredSandbox.join(' ')
  try {
    if (new URL(url, currentOrigin).origin !== currentOrigin) {
      return `${CHAT_IFRAME_SANDBOX} allow-same-origin`
    }
  } catch {
    // Invalid URLs remain in the stricter sandbox and fail in the iframe.
  }
  return CHAT_IFRAME_SANDBOX
}

export function postChatTheme(
  targetWindow: Window | null,
  theme: ChatTheme
): void {
  if (!targetWindow) return
  targetWindow.postMessage({ type: 'theme', value: theme }, '*')
}

export function parseChatConfigEntries(raw: RawChatConfig): ChatConfigEntry[] {
  let parsed: unknown = raw

  if (typeof raw === 'string') {
    try {
      parsed = JSON.parse(raw)
    } catch {
      return []
    }
  }

  if (!Array.isArray(parsed)) {
    return []
  }

  const result: ChatConfigEntry[] = []
  const names = new Set<string>()
  for (const entry of parsed) {
    if (!entry || typeof entry !== 'object' || Array.isArray(entry)) continue
    const config = entry as Record<string, unknown>
    if (
      !Object.keys(config).every((key) =>
        ['name', 'url', 'enabled', 'icon', 'open_mode', 'sandbox'].includes(key)
      ) ||
      typeof config.name !== 'string' ||
      typeof config.url !== 'string' ||
      typeof config.enabled !== 'boolean' ||
      (config.icon !== undefined &&
        (typeof config.icon !== 'string' ||
          (config.icon.trim() !== '' &&
            !CHAT_ICON_PATTERN.test(config.icon.trim()))))
    ) {
      continue
    }
    const name = config.name.trim()
    const url = config.url.trim()
    const icon = config.icon?.trim()
    if (!name || !url || names.has(name)) continue
    const isHttp = HTTP_REGEX.test(url)
    let openMode: ChatOpenMode | undefined
    if (isHttp) {
      if (
        config.open_mode !== undefined &&
        config.open_mode !== 'embedded' &&
        config.open_mode !== 'new_tab'
      ) {
        continue
      }
      openMode = (config.open_mode as ChatOpenMode | undefined) ?? 'embedded'
    } else if (config.open_mode !== undefined) {
      continue
    }
    let sandbox: ChatSandboxPermission[] | undefined
    if (config.sandbox !== undefined) {
      if (
        !isHttp ||
        openMode !== 'embedded' ||
        !Array.isArray(config.sandbox) ||
        config.sandbox.some(
          (permission) =>
            typeof permission !== 'string' ||
            !CHAT_SANDBOX_PERMISSION_SET.has(permission)
        ) ||
        new Set(config.sandbox).size !== config.sandbox.length
      ) {
        continue
      }
      sandbox = [...config.sandbox] as ChatSandboxPermission[]
    }
    names.add(name)
    result.push({
      name,
      url,
      enabled: config.enabled,
      ...(icon ? { icon } : {}),
      openMode,
      ...(sandbox !== undefined ? { sandbox } : {}),
    })
  }
  return result
}

export function serializeChatConfigEntry(entry: ChatConfigEntry) {
  return {
    name: entry.name,
    url: entry.url,
    enabled: entry.enabled,
    ...(entry.icon ? { icon: entry.icon } : {}),
    ...(entry.openMode ? { open_mode: entry.openMode } : {}),
    ...(entry.sandbox !== undefined ? { sandbox: [...entry.sandbox] } : {}),
  }
}

export function parseChatConfig(raw: RawChatConfig): ChatPreset[] {
  return parseChatConfigEntries(raw)
    .filter((entry) => entry.enabled)
    .map(
      (entry, index) =>
        ({
          id: String(index),
          name: entry.name,
          url: entry.url,
          type: detectChatLinkType(entry.url),
          ...(entry.icon ? { icon: entry.icon } : {}),
          openMode: entry.openMode,
          ...(entry.sandbox !== undefined
            ? { sandbox: [...entry.sandbox] }
            : {}),
        }) satisfies ChatPreset
    )
}

function replaceToken(source: string, token: string, value: string) {
  return source.split(token).join(value)
}

function normalizeApiKey(apiKey: string): string {
  const trimmed = apiKey.trim()
  if (!trimmed) return ''
  return trimmed.startsWith('sk-') ? trimmed : `sk-${trimmed}`
}

export function resolveChatUrl({
  template,
  apiKey,
  serverAddress,
  theme,
}: ResolveChatUrlParams): string {
  let url = replaceToken(template, '{theme}', theme)
  const safeServerAddress = serverAddress || ''

  const safeApiKey = normalizeApiKey(apiKey || '')

  if (url.includes('{cherryConfig}')) {
    const payload = {
      id: 'new-api',
      baseUrl: safeServerAddress,
      apiKey: safeApiKey,
    }
    const encoded = encodeURIComponent(toBase64(JSON.stringify(payload)))
    return replaceToken(url, '{cherryConfig}', encoded)
  }

  if (url.includes('{aionuiConfig}')) {
    const payload = {
      platform: 'new-api',
      baseUrl: safeServerAddress,
      apiKey: safeApiKey,
    }
    const encoded = encodeURIComponent(toBase64(JSON.stringify(payload)))
    return replaceToken(url, '{aionuiConfig}', encoded)
  }

  if (url.includes('{deepchatConfig}')) {
    const payload = {
      id: 'new-api',
      baseUrl: safeServerAddress,
      apiKey: safeApiKey,
    }
    const encoded = encodeURIComponent(toBase64(JSON.stringify(payload)))
    return replaceToken(url, '{deepchatConfig}', encoded)
  }

  if (url.includes('{aqbotConfig}')) {
    const query = [
      `name=${encodeURIComponent('New API')}`,
      `baseurl=${encodeURIComponent(safeServerAddress)}`,
      `apikey=${encodeURIComponent(safeApiKey)}`,
      'type=openai',
    ].join('&')
    return replaceToken(url, '{aqbotConfig}', query)
  }

  if (safeServerAddress) {
    const encodedAddress = encodeURIComponent(safeServerAddress)
    url = replaceToken(url, '{address}', encodedAddress)
  }

  if (safeApiKey) {
    url = replaceToken(url, '{key}', safeApiKey)
  }

  return url
}

export function getFirstActiveKey(
  keys: ActiveApiKey[] | undefined
): ActiveApiKey | undefined {
  if (!Array.isArray(keys)) return undefined
  return keys.find((item) => item.status === API_KEY_STATUS.ENABLED)
}

export function shouldGroupChatEntries(count: number, threshold: number) {
  return count > threshold
}
