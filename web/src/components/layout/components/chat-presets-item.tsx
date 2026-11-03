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
import { Link, useLocation, useNavigate } from '@tanstack/react-router'
import type { TFunction } from 'i18next'
import { ExternalLink, Loader2, ChevronRight } from 'lucide-react'
import { lazy, Suspense, useMemo, useCallback } from 'react'
import { useTranslation } from 'react-i18next'

import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import {
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarMenuSub,
  SidebarMenuSubButton,
  SidebarMenuSubItem,
  useSidebar,
} from '@/components/ui/sidebar'
import { useTheme } from '@/context/theme-provider'
import { useChatPresetLauncher } from '@/features/chat/components/chat-preset-launcher'
import { useChatPresets } from '@/features/chat/hooks/use-chat-presets'
import {
  chatLinkRequiresApiKey,
  chatLinkRequiresBackendLaunch,
  chatPresetOpensInNewTab,
  resolveChatUrl,
  shouldGroupChatEntries,
  type ChatPreset,
} from '@/features/chat/lib/chat-links'

import { normalizeHref } from '../lib/url-utils'
import type { NavChatPresets } from '../types'

const CustomChatPresetIcon = lazy(() =>
  import('@/features/chat/components/chat-preset-icon').then((module) => ({
    default: module.ChatPresetIcon,
  }))
)

function PresetMenuIcon(props: {
  name?: string
  fallback?: React.ElementType
}) {
  const Fallback = props.fallback
  if (!props.name) {
    return Fallback ? <Fallback aria-hidden='true' /> : null
  }

  const fallback = Fallback ? <Fallback aria-hidden='true' /> : null

  return (
    <Suspense fallback={fallback}>
      <CustomChatPresetIcon
        name={props.name}
        fallback={props.fallback}
        aria-hidden='true'
      />
    </Suspense>
  )
}

function isEmbeddedWebPreset(preset: ChatPreset): boolean {
  return preset.type === 'web' && !chatPresetOpensInNewTab(preset)
}

function chatPresetRequiresToken(preset: ChatPreset): boolean {
  return (
    chatLinkRequiresBackendLaunch(preset.url) ||
    chatLinkRequiresApiKey(preset.url)
  )
}

function getExternalPresetLabel(preset: ChatPreset, t: TFunction): string {
  if (preset.type === 'web') {
    return t('{{name}} (opens in a new tab)', { name: preset.name })
  }
  return t('{{name}} (opens an external app)', { name: preset.name })
}

/**
 * Sub-menu item for a single chat preset
 */
function ChatMenuItem({
  preset,
  active,
  loading,
  onOpen,
  onNavigate,
  preload,
}: {
  preset: ChatPreset
  active: boolean
  loading: boolean
  onOpen: (preset: ChatPreset) => void | Promise<void>
  onNavigate: () => void
  preload?: false
}) {
  const { t } = useTranslation()
  const embedded = isEmbeddedWebPreset(preset)
  if (embedded && !chatPresetRequiresToken(preset)) {
    return (
      <SidebarMenuSubItem>
        <SidebarMenuSubButton
          isActive={active}
          render={
            <Link
              to='/chat/$chatId'
              params={{ chatId: preset.id }}
              preload={preload}
              onClick={onNavigate}
            />
          }
        >
          <span className='min-w-0 flex-1 truncate whitespace-nowrap'>
            {preset.name}
          </span>
        </SidebarMenuSubButton>
      </SidebarMenuSubItem>
    )
  }

  return (
    <SidebarMenuSubItem>
      <SidebarMenuSubButton
        onClick={() => {
          if (!loading) void onOpen(preset)
        }}
        aria-disabled={loading ? 'true' : undefined}
        aria-label={embedded ? undefined : getExternalPresetLabel(preset, t)}
        isActive={embedded && active}
        className={embedded ? undefined : 'justify-between'}
      >
        <span className='min-w-0 flex-1 truncate whitespace-nowrap'>
          {preset.name}
        </span>
        {loading && (
          <Loader2
            aria-hidden='true'
            className='h-4 w-4 shrink-0 animate-spin'
          />
        )}
        {!loading && !embedded && (
          <ExternalLink aria-hidden='true' className='h-4 w-4 shrink-0' />
        )}
      </SidebarMenuSubButton>
    </SidebarMenuSubItem>
  )
}

function DirectPresetItem({
  preset,
  icon: Icon,
  active,
  loading,
  onOpen,
  onNavigate,
  preload,
}: {
  preset: ChatPreset
  icon?: React.ElementType
  active: boolean
  loading: boolean
  onOpen: (preset: ChatPreset) => void | Promise<void>
  onNavigate: () => void
  preload?: false
}) {
  const { t } = useTranslation()
  const embedded = isEmbeddedWebPreset(preset)
  if (embedded && !chatPresetRequiresToken(preset)) {
    return (
      <SidebarMenuItem>
        <SidebarMenuButton
          isActive={active}
          tooltip={preset.name}
          render={
            <Link
              to='/chat/$chatId'
              params={{ chatId: preset.id }}
              preload={preload}
              onClick={onNavigate}
            />
          }
        >
          <PresetMenuIcon name={preset.icon} fallback={Icon} />
          <span className='min-w-0 flex-1 truncate'>{preset.name}</span>
        </SidebarMenuButton>
      </SidebarMenuItem>
    )
  }

  return (
    <SidebarMenuItem>
      <SidebarMenuButton
        tooltip={preset.name}
        onClick={() => {
          if (!loading) void onOpen(preset)
        }}
        aria-disabled={loading ? 'true' : undefined}
        aria-label={embedded ? undefined : getExternalPresetLabel(preset, t)}
        isActive={embedded && active}
        className={embedded ? undefined : 'justify-between'}
      >
        <PresetMenuIcon name={preset.icon} fallback={Icon} />
        <span className='min-w-0 flex-1 truncate'>{preset.name}</span>
        {loading && (
          <Loader2
            aria-hidden='true'
            className='h-4 w-4 shrink-0 animate-spin'
          />
        )}
        {!loading && !embedded && (
          <ExternalLink aria-hidden='true' className='h-4 w-4 shrink-0' />
        )}
      </SidebarMenuButton>
    </SidebarMenuItem>
  )
}

/**
 * Dropdown menu item for a single chat preset
 */
function DropdownPresetItem({
  preset,
  loading,
  onOpen,
}: {
  preset: ChatPreset
  loading: boolean
  onOpen: (preset: ChatPreset) => void | Promise<void>
}) {
  const { t } = useTranslation()
  const embedded = isEmbeddedWebPreset(preset)
  if (embedded && !chatPresetRequiresToken(preset)) {
    return (
      <DropdownMenuItem
        render={<Link to='/chat/$chatId' params={{ chatId: preset.id }} />}
      >
        {preset.name}
      </DropdownMenuItem>
    )
  }

  return (
    <DropdownMenuItem
      disabled={loading}
      aria-label={embedded ? undefined : getExternalPresetLabel(preset, t)}
      onClick={() => {
        if (!loading) void onOpen(preset)
      }}
    >
      {preset.name}
      {loading && (
        <Loader2
          aria-hidden='true'
          className='ml-auto h-4 w-4 animate-spin opacity-70'
        />
      )}
      {!loading && !embedded && (
        <ExternalLink
          aria-hidden='true'
          className='ml-auto h-4 w-4 opacity-70'
        />
      )}
    </DropdownMenuItem>
  )
}

/**
 * Dynamic chat presets navigation item
 */
export function ChatPresetsItem({ item }: { item: NavChatPresets }) {
  const { chatPresets, serverAddress, chatMenuCollapseThreshold } =
    useChatPresets()
  const { resolvedTheme } = useTheme()
  const navigate = useNavigate()
  const { state, isMobile, setOpenMobile } = useSidebar()
  const href = useLocation({ select: (location) => location.href })

  const visiblePresets = useMemo(
    () => chatPresets.filter((preset) => preset.type !== 'fluent'),
    [chatPresets]
  )

  const handleLaunched = useCallback(
    (_preset: ChatPreset, launchURL: string) => {
      window.open(launchURL, '_blank', 'noopener')
      setOpenMobile(false)
    },
    [setOpenMobile]
  )
  const handleEmbeddedTokenSelected = useCallback(
    async (preset: ChatPreset, tokenId: number) => {
      setOpenMobile(false)
      await navigate({
        to: '/chat/$chatId',
        params: { chatId: preset.id },
        search: { tokenId },
      })
    },
    [navigate, setOpenMobile]
  )
  const presetLauncher = useChatPresetLauncher(
    handleLaunched,
    serverAddress,
    handleEmbeddedTokenSelected
  )
  const prepareLaunch = presetLauncher.prepareLaunch
  const loadingPresetId = presetLauncher.loadingPresetId

  const handleOpenPreset = useCallback(
    async (preset: ChatPreset) => {
      if (isEmbeddedWebPreset(preset) && !chatPresetRequiresToken(preset)) {
        return
      }

      if (chatPresetRequiresToken(preset)) {
        await prepareLaunch(preset)
        return
      }

      const url = resolveChatUrl({
        template: preset.url,
        serverAddress,
        theme: resolvedTheme,
      })

      if (typeof window === 'undefined') return

      window.open(url, '_blank', 'noopener')
      setOpenMobile(false)
    },
    [prepareLaunch, resolvedTheme, serverAddress, setOpenMobile]
  )

  const normalizedHref = normalizeHref(href)
  const shouldGroup = shouldGroupChatEntries(
    visiblePresets.length,
    chatMenuCollapseThreshold
  )

  // Don't render if no visible presets
  if (visiblePresets.length === 0) {
    return null
  }

  if (!shouldGroup) {
    return (
      <>
        {visiblePresets.map((preset) => (
          <DirectPresetItem
            key={preset.id}
            preset={preset}
            icon={item.icon}
            active={normalizedHref === `/chat/${preset.id}`}
            loading={loadingPresetId === preset.id}
            onOpen={handleOpenPreset}
            onNavigate={() => setOpenMobile(false)}
            preload={isMobile ? false : undefined}
          />
        ))}
        {presetLauncher.dialog}
      </>
    )
  }

  // Collapsed state on non-mobile - render dropdown menu
  if (state === 'collapsed' && !isMobile) {
    return (
      <>
        <SidebarMenuItem>
          <DropdownMenu>
            <DropdownMenuTrigger
              render={<SidebarMenuButton tooltip={item.title} />}
            >
              {item.icon && <item.icon className='h-4 w-4 shrink-0' />}
              <span className='min-w-0 flex-1 truncate'>{item.title}</span>
              <ChevronRight className='ms-auto h-4 w-4 shrink-0 opacity-70' />
            </DropdownMenuTrigger>
            <DropdownMenuContent align='start'>
              {visiblePresets.map((preset) => (
                <DropdownPresetItem
                  key={preset.id}
                  preset={preset}
                  loading={loadingPresetId === preset.id}
                  onOpen={handleOpenPreset}
                />
              ))}
            </DropdownMenuContent>
          </DropdownMenu>
        </SidebarMenuItem>
        {presetLauncher.dialog}
      </>
    )
  }

  // Expanded state - render collapsible menu
  return (
    <>
      <Collapsible
        defaultOpen={normalizedHref.startsWith('/chat')}
        className='group/collapsible'
        render={<SidebarMenuItem />}
      >
        <CollapsibleTrigger
          className='group/collapsible-trigger'
          render={<SidebarMenuButton />}
        >
          {item.icon && <item.icon className='shrink-0' />}
          <span className='min-w-0 flex-1 truncate'>{item.title}</span>
          <ChevronRight className='ms-auto size-4 shrink-0 transition-transform duration-200 group-data-[panel-open]/collapsible-trigger:rotate-90' />
        </CollapsibleTrigger>
        <CollapsibleContent className='CollapsibleContent'>
          <SidebarMenuSub>
            {visiblePresets.map((preset) => (
              <ChatMenuItem
                key={preset.id}
                preset={preset}
                active={normalizedHref === `/chat/${preset.id}`}
                loading={loadingPresetId === preset.id}
                onOpen={handleOpenPreset}
                onNavigate={() => setOpenMobile(false)}
                preload={isMobile ? false : undefined}
              />
            ))}
          </SidebarMenuSub>
        </CollapsibleContent>
      </Collapsible>
      {presetLauncher.dialog}
    </>
  )
}
