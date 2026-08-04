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
import { Search01Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useState } from 'react'
import type { UseFormReturn } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import {
  Field,
  FieldDescription,
  FieldError,
  FieldLabel,
} from '@/components/ui/field'
import { Spinner } from '@/components/ui/spinner'
import { Textarea } from '@/components/ui/textarea'

import type { ChannelDiscoveryConnection } from '../channel-setup'
import {
  discoveryTextSchema,
  channelSetupLayoutClasses,
  type DiscoveryFormValues,
} from '../lib/channel-discovery-form'

type ChannelDiscoverySourcePanelProps = {
  form: UseFormReturn<DiscoveryFormValues>
  connections: ChannelDiscoveryConnection[]
  totalCount: number
  completedCount: number
  draftedBlockIndexes: number[]
  selectedBlockIndex: number | null
  discovering: boolean
  onTextChange: () => void
  onReset: () => void
  onDiscover: (text: string) => void
  onSelect: (connection: ChannelDiscoveryConnection) => void
}

/**
 * Accepts connection blocks and renders their redacted discovery results.
 * @param props - Source form, discovery state, results, and selection callbacks.
 * @returns The source-input and connection-selection panel.
 */
export function ChannelDiscoverySourcePanel(
  props: ChannelDiscoverySourcePanelProps
) {
  const { t } = useTranslation()
  const [resetOpen, setResetOpen] = useState(false)
  const locked = props.totalCount > 0

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('Connection blocks')}</CardTitle>
        <CardDescription>
          {t(
            'Paste text containing upstream URLs and API keys in either order. Separate multiple connections with a blank line.'
          )}
        </CardDescription>
      </CardHeader>
      <CardContent className='space-y-4'>
        <Field data-invalid={Boolean(props.form.formState.errors.text)}>
          <FieldLabel htmlFor='channel-discovery-input'>
            {t('Upstream URLs and API keys')}
          </FieldLabel>
          <Textarea
            id='channel-discovery-input'
            className='min-h-44 resize-y'
            placeholder={'https://api.example.com/v1\nsk-...'}
            aria-invalid={Boolean(props.form.formState.errors.text)}
            readOnly={locked}
            {...props.form.register('text', { onChange: props.onTextChange })}
          />
          <FieldError>
            {props.form.formState.errors.text?.message
              ? t(props.form.formState.errors.text.message)
              : null}
          </FieldError>
          <FieldDescription>
            {t(
              'Keys are sent only to the origin in their connection block and are never returned by the API.'
            )}
          </FieldDescription>
        </Field>
        <Button
          className={`w-full ${channelSetupLayoutClasses.mobileAction}`}
          onClick={() => {
            const parsed = discoveryTextSchema.safeParse(
              props.form.getValues('text')
            )
            if (!parsed.success) {
              props.form.setError('text', {
                message: parsed.error.issues[0]?.message,
              })
              return
            }
            props.form.clearErrors('text')
            props.onDiscover(parsed.data)
          }}
          disabled={props.discovering || locked}
        >
          {props.discovering ? (
            <Spinner aria-label={t('Discovering channels')} />
          ) : (
            <HugeiconsIcon icon={Search01Icon} strokeWidth={2} />
          )}
          {props.discovering ? t('Discovering...') : t('Discover connections')}
        </Button>

        {locked && (
          <div className='flex items-center justify-between gap-2'>
            <Badge variant='outline'>
              {t('{{completed}} of {{total}} completed', {
                completed: props.completedCount,
                total: props.totalCount,
              })}
            </Badge>
            <Button
              type='button'
              variant='ghost'
              size='sm'
              onClick={() => setResetOpen(true)}
            >
              {t('Reset')}
            </Button>
          </div>
        )}

        {props.connections.length > 0 && (
          <div className='space-y-2' aria-label={t('Discovery results')}>
            {props.connections.map((connection) => {
              let status = t('Failed')
              if (connection.usable) {
                status = props.draftedBlockIndexes.includes(
                  connection.block_index
                )
                  ? t('Draft')
                  : t('Usable')
              }
              return (
                <button
                  type='button'
                  key={connection.block_index}
                  className={`focus-visible:ring-ring hover:bg-muted/50 w-full rounded-lg border p-3 text-left outline-none focus-visible:ring-3 disabled:opacity-60 ${channelSetupLayoutClasses.discoveryResult}`}
                  data-selected={
                    connection.block_index === props.selectedBlockIndex
                  }
                  aria-pressed={
                    connection.block_index === props.selectedBlockIndex
                  }
                  disabled={!connection.usable}
                  onClick={() => props.onSelect(connection)}
                >
                  <span className='flex items-center justify-between gap-2'>
                    <span className='truncate font-medium'>
                      {connection.suggested_name}
                    </span>
                    <Badge
                      variant={connection.usable ? 'secondary' : 'destructive'}
                    >
                      {status}
                    </Badge>
                  </span>
                  <span className='text-muted-foreground mt-1 block truncate text-xs'>
                    {connection.base_url}
                  </span>
                  <span className='text-muted-foreground mt-2 block text-xs'>
                    {t('{{models}} models · {{keys}} usable keys', {
                      models: connection.models.length,
                      keys: connection.usable_key_indexes.length,
                    })}
                  </span>
                  {connection.error_message && (
                    <span className='text-destructive mt-2 block text-xs'>
                      {connection.error_message}
                    </span>
                  )}
                </button>
              )
            })}
          </div>
        )}
      </CardContent>
      <AlertDialog open={resetOpen} onOpenChange={setResetOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t('Reset')}</AlertDialogTitle>
            <AlertDialogDescription>
              {t(
                'Reset connection discovery? Unsaved drafts will be discarded.'
              )}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t('Cancel')}</AlertDialogCancel>
            <AlertDialogAction variant='destructive' onClick={props.onReset}>
              {t('Reset')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </Card>
  )
}
