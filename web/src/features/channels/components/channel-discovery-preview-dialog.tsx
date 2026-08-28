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
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'

import type { ChannelDiscoveryPreview } from '../channel-setup'
import { channelSetupLayoutClasses } from '../lib/channel-discovery-form'

const changeLabels: Record<string, string> = {
  create: 'Create Channel',
  keys: 'API Keys',
  models: 'Models',
  mapping: 'Model Mapping',
  routes: 'Endpoints',
  status: 'Status',
  type: 'Type',
  base_url: 'Base URL',
}

type ChannelDiscoveryPreviewDialogProps = {
  open: boolean
  preview: ChannelDiscoveryPreview | null
  applying: boolean
  canApply: boolean
  onOpenChange: (open: boolean) => void
  onConfirm: () => void
}

/**
 * Renders the immutable server preview and confirms its application.
 * @param props - Preview state, permission state, and confirmation callbacks.
 * @returns A controlled review dialog.
 */
export function ChannelDiscoveryPreviewDialog(
  props: ChannelDiscoveryPreviewDialogProps
) {
  const { t } = useTranslation()

  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      <DialogContent
        className={`max-h-[calc(100dvh-2rem)] overflow-y-auto sm:max-w-2xl ${channelSetupLayoutClasses.mobileControls}`}
      >
        <DialogHeader>
          <DialogTitle>{t('Confirm channel configuration')}</DialogTitle>
          <DialogDescription>
            {t('Review the exact changes before they are applied.')}
          </DialogDescription>
        </DialogHeader>
        {props.preview && (
          <div className='space-y-4'>
            <dl className='bg-muted/50 grid gap-3 rounded-lg p-3 sm:grid-cols-2'>
              <div>
                <dt className='text-muted-foreground text-xs'>{t('Action')}</dt>
                <dd className='font-medium'>
                  {t(
                    props.preview.operation === 'create' ? 'Create' : 'Update'
                  )}
                </dd>
              </div>
              <div>
                <dt className='text-muted-foreground text-xs'>{t('Type')}</dt>
                <dd className='font-medium'>{t(props.preview.type_name)}</dd>
              </div>
              <div className='sm:col-span-2'>
                <dt className='text-muted-foreground text-xs'>{t('Name')}</dt>
                <dd className='font-medium break-words'>
                  {props.preview.name}
                </dd>
              </div>
              <div className='sm:col-span-2'>
                <dt className='text-muted-foreground text-xs'>
                  {t('Base URL')}
                </dt>
                <dd className='font-medium break-all'>
                  {props.preview.base_url}
                </dd>
              </div>
              <div>
                <dt className='text-muted-foreground text-xs'>{t('Groups')}</dt>
                <dd className='font-medium'>
                  {props.preview.groups.join(', ')}
                </dd>
              </div>
              <div>
                <dt className='text-muted-foreground text-xs'>
                  {t('API Keys')}
                </dt>
                <dd className='font-medium'>{props.preview.key_count}</dd>
              </div>
              <div>
                <dt className='text-muted-foreground text-xs'>{t('Status')}</dt>
                <dd className='font-medium'>
                  {props.preview.status === 1 ? t('Enabled') : t('Disabled')}
                </dd>
              </div>
              <div>
                <dt className='text-muted-foreground text-xs'>
                  {t('Priority')}
                </dt>
                <dd className='font-medium'>{props.preview.priority}</dd>
              </div>
              <div className='sm:col-span-2'>
                <dt className='text-muted-foreground text-xs'>{t('Tag')}</dt>
                <dd className='font-medium'>{props.preview.tag || '—'}</dd>
              </div>
              <div className='sm:col-span-2'>
                <dt className='text-muted-foreground text-xs'>
                  {t('Changed Fields')}
                </dt>
                <dd className='mt-1 flex flex-wrap gap-1.5'>
                  {props.preview.changes.map((change) => (
                    <Badge key={change} variant='secondary'>
                      {t(changeLabels[change] ?? change)}
                    </Badge>
                  ))}
                </dd>
              </div>
            </dl>

            <section className='space-y-2'>
              <h3 className='text-sm font-medium'>
                {t('Models')} ({props.preview.models.length})
              </h3>
              <div className='flex max-h-36 flex-wrap gap-1.5 overflow-y-auto rounded-lg border p-2'>
                {props.preview.models.map((model) => (
                  <Badge key={model} variant='outline'>
                    {model}
                  </Badge>
                ))}
              </div>
            </section>

            <section className='space-y-2'>
              <h3 className='text-sm font-medium'>{t('Model Mapping')}</h3>
              <div className='space-y-1.5 rounded-lg border p-2'>
                {Object.keys(props.preview.mapping).length === 0 ? (
                  <p className='text-muted-foreground text-sm'>
                    {t('No model mapping changes')}
                  </p>
                ) : (
                  Object.entries(props.preview.mapping).map(
                    ([source, target]) => (
                      <div
                        key={source}
                        className='grid gap-1 text-sm sm:grid-cols-[minmax(0,1fr)_auto_minmax(0,1fr)] sm:items-center'
                      >
                        <span className='break-all'>{source}</span>
                        <span className='text-muted-foreground max-sm:hidden'>
                          →
                        </span>
                        <span className='font-medium break-all'>{target}</span>
                      </div>
                    )
                  )
                )}
              </div>
            </section>

            <section className='space-y-2'>
              <h3 className='text-sm font-medium'>{t('Endpoints')}</h3>
              <div className='space-y-1.5 rounded-lg border p-2'>
                {props.preview.routes.map((route) => (
                  <div
                    key={route.protocol}
                    className='grid gap-1 text-sm sm:grid-cols-[7rem_minmax(0,1fr)]'
                  >
                    <span className='font-medium'>{route.protocol}</span>
                    <span className='text-muted-foreground break-all'>
                      {route.incoming_path} → {route.upstream_path}
                    </span>
                  </div>
                ))}
              </div>
            </section>
          </div>
        )}
        <DialogFooter>
          <Button
            variant='outline'
            onClick={() => props.onOpenChange(false)}
            disabled={props.applying}
          >
            {t('Cancel')}
          </Button>
          <Button
            onClick={props.onConfirm}
            disabled={props.applying || !props.canApply}
          >
            {props.applying ? t('Applying...') : t('Apply configuration')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
