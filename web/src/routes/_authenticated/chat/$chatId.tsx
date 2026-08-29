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
import { Link, createFileRoute, redirect } from '@tanstack/react-router'
import { Loader2, MessageCircleWarning } from 'lucide-react'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import z from 'zod'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { useTheme } from '@/context/theme-provider'
import { useChatPresetLauncher } from '@/features/chat/components/chat-preset-launcher'
import { useChatPresets } from '@/features/chat/hooks/use-chat-presets'
import {
  chatLinkRequiresApiKey,
  chatLinkRequiresBackendLaunch,
  chatPresetOpensInNewTab,
  getChatIframeSandbox,
  postChatTheme,
  resolveChatUrl,
  type ChatPreset,
} from '@/features/chat/lib/chat-links'

const chatSearchSchema = z.object({
  tokenId: z.coerce.number().int().positive().optional().catch(undefined),
})

export const Route = createFileRoute('/_authenticated/chat/$chatId')({
  validateSearch: chatSearchSchema,
  loader: async ({ params }) => {
    if (!Number.isInteger(Number(params.chatId))) {
      throw redirect({ to: '/dashboard' })
    }
  },
  component: ChatRouteComponent,
})

function ChatRouteComponent() {
  const { t } = useTranslation()
  const { resolvedTheme } = useTheme()
  const { chatId } = Route.useParams()
  const { tokenId } = Route.useSearch()
  const navigate = Route.useNavigate()
  const { chatPresets, serverAddress } = useChatPresets()
  const preset = useMemo(() => {
    const index = Number(chatId)
    if (!Number.isInteger(index)) return undefined
    return chatPresets[index]
  }, [chatId, chatPresets])

  const isEmbeddedWebLink =
    preset?.type === 'web' && !chatPresetOpensInNewTab(preset)
  const requiresToken = Boolean(
    preset &&
    isEmbeddedWebLink &&
    (chatLinkRequiresBackendLaunch(preset.url) ||
      chatLinkRequiresApiKey(preset.url))
  )

  const [preparedIframeSrc, setPreparedIframeSrc] = useState('')
  const [launchTheme] = useState(resolvedTheme)
  const iframeRef = useRef<HTMLIFrameElement>(null)
  const launchedPresetRef = useRef<string | null>(null)
  const handleLaunched = useCallback(
    (_preset: ChatPreset, launchURL: string) => {
      setPreparedIframeSrc(launchURL)
      if (tokenId) {
        void navigate({
          to: '/chat/$chatId',
          params: { chatId },
          search: {},
          replace: true,
        })
      }
    },
    [chatId, navigate, tokenId]
  )
  const handleLaunchAborted = useCallback(() => {
    void navigate({ to: '/dashboard', replace: true })
  }, [navigate])
  const presetLauncher = useChatPresetLauncher(
    handleLaunched,
    serverAddress,
    undefined,
    handleLaunchAborted
  )
  const prepareLaunch = presetLauncher.prepareLaunch
  const launchWithTokenId = presetLauncher.launchWithTokenId

  useEffect(() => {
    if (!preset || !requiresToken) return
    const launchKey = `${preset.id}:${tokenId ?? 'choose'}`
    if (launchedPresetRef.current === launchKey) return
    launchedPresetRef.current = launchKey
    setPreparedIframeSrc('')
    if (tokenId) {
      void launchWithTokenId(preset, tokenId).then((success) => {
        if (!success) void prepareLaunch(preset)
      })
      return
    }
    void prepareLaunch(preset)
  }, [launchWithTokenId, prepareLaunch, preset, requiresToken, tokenId])

  const iframeSrc = useMemo(() => {
    if (!preset || !isEmbeddedWebLink) return ''
    if (requiresToken) return preparedIframeSrc
    return resolveChatUrl({
      template: preset.url,
      serverAddress,
      theme: launchTheme,
    })
  }, [
    isEmbeddedWebLink,
    launchTheme,
    preset,
    preparedIframeSrc,
    requiresToken,
    serverAddress,
  ])

  const syncIframeTheme = useCallback(() => {
    if (!iframeSrc) return
    postChatTheme(iframeRef.current?.contentWindow ?? null, resolvedTheme)
  }, [iframeSrc, resolvedTheme])

  useEffect(syncIframeTheme, [syncIframeTheme])

  if (!preset) {
    return (
      <div className='flex h-full flex-col items-center justify-center gap-4 p-6 text-center'>
        <MessageCircleWarning className='text-muted-foreground h-12 w-12' />
        <div className='space-y-1'>
          <h2 className='text-lg font-semibold'>
            {t('Chat preset not found')}
          </h2>
          <p className='text-muted-foreground'>
            {t('The requested chat preset does not exist or has been removed.')}
          </p>
        </div>
        <Button variant='outline' render={<Link to='/dashboard' />}>
          {t('Return to dashboard')}
        </Button>
      </div>
    )
  }

  if (!isEmbeddedWebLink) {
    return (
      <div className='flex h-full flex-col items-center justify-center gap-4 p-6 text-center'>
        <MessageCircleWarning className='text-muted-foreground h-12 w-12' />
        <div className='space-y-1'>
          <h2 className='text-lg font-semibold'>{t('Use sidebar shortcut')}</h2>
          <p className='text-muted-foreground'>
            {preset.name}{' '}
            {t(
              'opens in an external client. Trigger it from the sidebar or API key actions to launch the configured application.'
            )}
          </p>
        </div>
        <Button variant='outline' render={<Link to='/dashboard' />}>
          {t('Return to dashboard')}
        </Button>
      </div>
    )
  }

  if (requiresToken && !iframeSrc) {
    return (
      <>
        <div
          role='status'
          aria-label={t('Preparing your chat link…')}
          className='flex h-full items-center justify-center p-6'
        >
          <Loader2
            aria-hidden='true'
            className='text-muted-foreground size-9 animate-spin'
          />
        </div>
        {presetLauncher.dialog}
      </>
    )
  }

  if (!iframeSrc) {
    return (
      <div className='flex h-full flex-col items-center justify-center p-6'>
        <Alert variant='destructive' className='max-w-xl'>
          <AlertTitle>{t('Unable to open chat')}</AlertTitle>
          <AlertDescription>
            {t(
              'Unable to generate chat link. Please contact your administrator.'
            )}
          </AlertDescription>
        </Alert>
      </div>
    )
  }

  return (
    <iframe
      ref={iframeRef}
      src={iframeSrc}
      key={iframeSrc}
      className='h-full w-full border-0'
      allow='camera; microphone'
      sandbox={getChatIframeSandbox(
        iframeSrc,
        window.location.origin,
        preset.sandbox
      )}
      title={t('Chat preset: {{name}}', { name: preset.name })}
      onLoad={syncIframeTheme}
    />
  )
}
