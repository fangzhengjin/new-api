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
import { Link } from '@tanstack/react-router'
import { KeyRound, Loader2 } from 'lucide-react'
import { useCallback, useId, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Label } from '@/components/ui/label'
import { useTheme } from '@/context/theme-provider'
import type { ApiKey } from '@/features/keys/types'
import { formatTimestampToDate } from '@/lib/format'
import { getServerErrorMessage } from '@/lib/handle-server-error'
import { useAuthStore } from '@/stores/auth-store'

import { isApiKeyUsable, launchChatPreset, loadAllApiKeys } from '../api'
import { fetchChatKey } from '../hooks/use-active-chat-key'
import {
  chatLinkRequiresApiKey,
  chatLinkRequiresBackendLaunch,
  chatPresetOpensInNewTab,
  resolveChatUrl,
  type ChatPreset,
} from '../lib/chat-links'

type ChatPresetTokenDialogProps = {
  open: boolean
  tokens: ApiKey[]
  launchingTokenId: number | null
  rememberChoice: boolean
  onOpenChange: (open: boolean) => void
  onRememberChoiceChange: (remember: boolean) => void
  onSelect: (token: ApiKey) => void
}

export function ChatPresetTokenDialog(props: ChatPresetTokenDialogProps) {
  const { t } = useTranslation()
  const rememberId = useId()

  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      <DialogContent className='sm:max-w-lg'>
        <DialogHeader>
          <DialogTitle>{t('Choose a Token')}</DialogTitle>
          <DialogDescription>
            {t('This Token will be used to open the selected chat preset.')}
          </DialogDescription>
        </DialogHeader>
        {props.tokens.length > 0 ? (
          <div className='max-h-[60vh] space-y-2 overflow-y-auto pr-1'>
            {props.tokens.map((token) => {
              const models =
                token.model_limits_enabled && token.model_limits
                  ? token.model_limits
                  : t('All models')
              const expires =
                token.expired_time === -1
                  ? t('Never expires')
                  : formatTimestampToDate(token.expired_time)
              return (
                <Button
                  key={token.id}
                  type='button'
                  variant='outline'
                  className='h-auto w-full justify-start px-3 py-3 text-left'
                  disabled={props.launchingTokenId !== null}
                  onClick={() => props.onSelect(token)}
                >
                  <span className='min-w-0 flex-1 space-y-1'>
                    <span className='block truncate font-medium'>
                      {token.name}
                    </span>
                    <span className='text-muted-foreground block truncate font-mono text-xs'>
                      {token.key}
                    </span>
                    <span className='text-muted-foreground block truncate text-xs'>
                      {t('Models')}: {models}
                    </span>
                    <span className='text-muted-foreground block text-xs'>
                      {t('Expires at')}: {expires}
                    </span>
                  </span>
                  {props.launchingTokenId === token.id && (
                    <Loader2
                      aria-hidden='true'
                      className='ml-2 size-4 shrink-0 animate-spin'
                    />
                  )}
                </Button>
              )
            })}
          </div>
        ) : (
          <div className='flex flex-col items-center gap-3 py-6 text-center'>
            <KeyRound
              aria-hidden='true'
              className='text-muted-foreground size-9'
            />
            <div className='space-y-1'>
              <p className='font-medium'>{t('No usable Token')}</p>
              <p className='text-muted-foreground text-sm'>
                {t('Create or enable a Token before opening this preset.')}
              </p>
            </div>
            <Button render={<Link to='/keys' />}>{t('Go to Tokens')}</Button>
          </div>
        )}
        {props.tokens.length > 0 && (
          <div className='border-border flex items-center gap-2 border-t pt-3'>
            <Checkbox
              id={rememberId}
              checked={props.rememberChoice}
              disabled={props.launchingTokenId !== null}
              onCheckedChange={(checked) =>
                props.onRememberChoiceChange(checked === true)
              }
            />
            <Label htmlFor={rememberId} className='text-sm font-normal'>
              {t('Remember my choice')}
            </Label>
          </div>
        )}
      </DialogContent>
    </Dialog>
  )
}

function tokenStorageKey(userId: number | undefined, presetName: string) {
  return userId
    ? `chat-preset-token:v1:${userId}:${encodeURIComponent(presetName)}`
    : null
}

function readRememberedTokenId(storageKey: string | null) {
  if (!storageKey) return null
  try {
    const tokenId = Number(window.localStorage.getItem(storageKey))
    return Number.isInteger(tokenId) && tokenId > 0 ? tokenId : null
  } catch {
    return null
  }
}

function writeRememberedTokenId(storageKey: string | null, tokenId: number) {
  if (!storageKey) return true
  try {
    window.localStorage.setItem(storageKey, String(tokenId))
    return true
  } catch {
    return false
  }
}

function clearRememberedTokenId(storageKey: string | null) {
  if (!storageKey) return true
  try {
    window.localStorage.removeItem(storageKey)
    return true
  } catch {
    return false
  }
}

// eslint-disable-next-line react-refresh/only-export-components
export function useChatPresetLauncher(
  onLaunched: (preset: ChatPreset, launchURL: string) => void | Promise<void>,
  serverAddress: string,
  onEmbeddedTokenSelected?: (
    preset: ChatPreset,
    tokenId: number
  ) => void | Promise<void>,
  onLaunchAborted?: () => void
) {
  const { t } = useTranslation()
  const { resolvedTheme } = useTheme()
  const userId = useAuthStore((store) => store.auth.user?.id)
  const [activePreset, setActivePreset] = useState<ChatPreset | null>(null)
  const [tokens, setTokens] = useState<ApiKey[]>([])
  const [dialogOpen, setDialogOpen] = useState(false)
  const [rememberChoice, setRememberChoice] = useState(false)
  const [loadingPresetId, setLoadingPresetId] = useState<string | null>(null)
  const [launchingTokenId, setLaunchingTokenId] = useState<number | null>(null)
  const loadingRef = useRef(false)

  const requestLaunchURL = useCallback(
    async (preset: ChatPreset, tokenId: number) => {
      setLaunchingTokenId(tokenId)
      try {
        const template = chatLinkRequiresBackendLaunch(preset.url)
          ? await launchChatPreset(preset.name, tokenId)
          : preset.url
        const apiKey = chatLinkRequiresApiKey(preset.url)
          ? await fetchChatKey(tokenId)
          : undefined
        return resolveChatUrl({
          template,
          apiKey,
          serverAddress,
          theme: resolvedTheme,
        })
      } catch (error) {
        toast.error(
          t('Unable to open chat preset: {{reason}}', {
            reason: getServerErrorMessage(error),
          })
        )
        return null
      } finally {
        setLaunchingTokenId(null)
      }
    },
    [resolvedTheme, serverAddress, t]
  )

  const launch = useCallback(
    async (preset: ChatPreset, token: ApiKey, shouldRemember?: boolean) => {
      const storageKey = tokenStorageKey(userId, preset.name)
      if (shouldRemember !== undefined) {
        const storageUpdated = shouldRemember
          ? writeRememberedTokenId(storageKey, token.id)
          : clearRememberedTokenId(storageKey)
        if (!storageUpdated) {
          toast.error(t('The Token was selected, but could not be remembered.'))
        }
      }
      if (
        onEmbeddedTokenSelected &&
        preset.type === 'web' &&
        !chatPresetOpensInNewTab(preset)
      ) {
        await onEmbeddedTokenSelected(preset, token.id)
        setDialogOpen(false)
        return
      }
      const launchURL = await requestLaunchURL(preset, token.id)
      if (!launchURL) {
        clearRememberedTokenId(storageKey)
        setDialogOpen(true)
        return
      }
      await onLaunched(preset, launchURL)
      setDialogOpen(false)
    },
    [onEmbeddedTokenSelected, onLaunched, requestLaunchURL, t, userId]
  )

  const launchWithTokenId = useCallback(
    async (preset: ChatPreset, tokenId: number) => {
      const launchURL = await requestLaunchURL(preset, tokenId)
      if (!launchURL) return false
      await onLaunched(preset, launchURL)
      setDialogOpen(false)
      return true
    },
    [onLaunched, requestLaunchURL]
  )

  const prepareLaunch = useCallback(
    async (preset: ChatPreset) => {
      if (loadingRef.current) {
        toast.info(t('Preparing your chat link, please try again in a moment.'))
        return
      }
      loadingRef.current = true
      setLoadingPresetId(preset.id)
      setActivePreset(preset)
      setRememberChoice(false)
      try {
        const usableTokens = (await loadAllApiKeys()).filter((token) =>
          isApiKeyUsable(token)
        )
        setTokens(usableTokens)
        const storageKey = tokenStorageKey(userId, preset.name)
        const rememberedId = readRememberedTokenId(storageKey)
        const remembered = usableTokens.find(
          (token) => token.id === rememberedId
        )
        if (remembered) {
          await launch(preset, remembered)
          return
        }
        if (rememberedId !== null) clearRememberedTokenId(storageKey)
        if (usableTokens.length === 1) {
          await launch(preset, usableTokens[0])
          return
        }
        setDialogOpen(true)
      } catch (error) {
        toast.error(
          t('Unable to prepare chat preset: {{reason}}', {
            reason: getServerErrorMessage(error),
          })
        )
        onLaunchAborted?.()
      } finally {
        loadingRef.current = false
        setLoadingPresetId(null)
      }
    },
    [launch, onLaunchAborted, t, userId]
  )

  return {
    loadingPresetId,
    launchWithTokenId,
    prepareLaunch,
    dialog: (
      <ChatPresetTokenDialog
        open={dialogOpen}
        tokens={tokens}
        launchingTokenId={launchingTokenId}
        rememberChoice={rememberChoice}
        onOpenChange={(open) => {
          if (launchingTokenId !== null) return
          setDialogOpen(open)
          if (!open) onLaunchAborted?.()
        }}
        onRememberChoiceChange={setRememberChoice}
        onSelect={(token) => {
          if (activePreset) void launch(activePreset, token, rememberChoice)
        }}
      />
    ),
  }
}
