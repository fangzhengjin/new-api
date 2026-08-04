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
import {
  AiScanIcon,
  Alert02Icon,
  CheckmarkCircle02Icon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import type { UseFormReturn } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Field,
  FieldDescription,
  FieldError,
  FieldLabel,
} from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { Separator } from '@/components/ui/separator'
import { Spinner } from '@/components/ui/spinner'
import { Switch } from '@/components/ui/switch'
import { cn } from '@/lib/utils'

import type {
  ChannelDiscoveryChoice,
  ChannelDiscoveryConnection,
} from '../channel-setup'
import {
  channelSetupLayoutClasses,
  type DiscoveryFormValues,
} from '../lib/channel-discovery-form'
import {
  ChannelDiscoveryEndpoints,
  ChannelDiscoveryOptions,
} from './channel-discovery-options'

type ChannelDiscoveryWorkspaceProps = {
  form: UseFormReturn<DiscoveryFormValues>
  connection: ChannelDiscoveryConnection
  groupOptions: string[]
  groups: string[]
  selectedModels: string[]
  mapping: Record<string, string>
  routes: Record<string, string>
  acceptedKeyIndexes: number[]
  probeModel: string
  probing: boolean
  probeStatus: 'idle' | 'probing' | 'success' | 'error'
  probeDisplayModel: string
  probeKeyIndex: number
  onGroupsChange: (groups: string[]) => void
  onAcceptedKeysChange: (indexes: number[]) => void
  onProbeModelChange: (model: string) => void
  onProbe: () => void
  onToggleModel: (choice: ChannelDiscoveryChoice, checked: boolean) => void
  onModelsChange: (choices: ChannelDiscoveryChoice[]) => void
  onSetMapping: (source: string, target: string) => void
  onRemoveMapping: (source: string) => void
  onSetRoute: (protocol: string, path: string | null) => void
}

type ProtocolProbeStatusProps = {
  status: ChannelDiscoveryWorkspaceProps['probeStatus']
  model: string
  keyIndex: number
  routes: Record<string, string>
}

function ProtocolProbeStatus(props: ProtocolProbeStatusProps) {
  const { t } = useTranslation()
  const protocols = Object.keys(props.routes).filter(
    (protocol) => protocol !== 'models'
  )
  const isError = props.status === 'error'
  const isSuccess = props.status === 'success'
  let statusIcon = AiScanIcon
  let statusLabel = t('Not probed yet')
  if (props.status === 'probing') {
    statusLabel = t('Probing {{model}} with key {{number}}', {
      model: props.model,
      number: props.keyIndex + 1,
    })
  } else if (isSuccess) {
    statusIcon = CheckmarkCircle02Icon
    statusLabel = t('Probe completed for {{model}}', { model: props.model })
  } else if (isError) {
    statusIcon = Alert02Icon
    statusLabel = t('Probe failed for {{model}}', { model: props.model })
  }

  return (
    <div
      role='status'
      aria-live='polite'
      aria-atomic='true'
      className={cn(
        'bg-muted/40 rounded-lg border px-3 py-2.5',
        isSuccess && 'border-success/40 bg-success/10',
        isError && 'border-destructive/40 bg-destructive/5'
      )}
    >
      <div className='flex min-w-0 items-start gap-2.5'>
        {props.status === 'probing' ? (
          <Spinner
            className='mt-0.5 shrink-0'
            role='presentation'
            aria-hidden='true'
          />
        ) : (
          <HugeiconsIcon
            icon={statusIcon}
            strokeWidth={2}
            aria-hidden='true'
            className={cn(
              'text-muted-foreground mt-0.5 size-4 shrink-0',
              isSuccess && 'text-success',
              isError && 'text-destructive'
            )}
          />
        )}
        <div className='min-w-0 space-y-1.5'>
          <p className='text-sm font-medium break-all'>
            {statusLabel}
          </p>
          {isSuccess && (
            <div className='flex flex-wrap gap-1.5'>
              {protocols.map((protocol) => (
                <Badge key={protocol} variant='secondary'>
                  {protocol.replaceAll('_', ' ')}
                </Badge>
              ))}
              {protocols.length === 0 && (
                <Badge variant='destructive'>
                  {t('No protocol detected')}
                </Badge>
              )}
            </div>
          )}
          {isError && (
            <p className='text-muted-foreground text-xs'>
              {t(
                'Try again after checking the Base URL, API key, and upstream protocol support.'
              )}
            </p>
          )}
        </div>
      </div>
    </div>
  )
}

/**
 * Renders editable channel settings for the selected discovery result.
 * @param props - Current draft state, permissions, and editing callbacks.
 * @returns The channel configuration workspace.
 */
export function ChannelDiscoveryWorkspace(
  props: ChannelDiscoveryWorkspaceProps
) {
  const { t } = useTranslation()
  const operation = props.form.watch('operation')
  const syncConfiguration = props.form.watch('syncConfiguration')
  const showConfiguration = operation === 'create' || syncConfiguration

  return (
    <fieldset
      disabled={props.probing}
      aria-busy={props.probing}
      className='min-w-0 border-0 p-0'
    >
      <Card>
        <CardHeader>
          <CardTitle className='flex flex-wrap items-center gap-2'>
            <span>{props.connection.suggested_name}</span>
            <Badge variant='outline'>
              {t('{{count}} models', { count: props.connection.models.length })}
            </Badge>
            {props.connection.matches.length > 0 && (
              <Badge variant='warning'>
                {t('{{count}} matching channels', {
                  count: props.connection.matches.length,
                })}
              </Badge>
            )}
          </CardTitle>
          <CardDescription className='break-all'>
            {props.connection.base_url}
          </CardDescription>
        </CardHeader>
        <CardContent className='space-y-5'>
          {showConfiguration && (
            <>
              <section className='space-y-3'>
                <div>
                  <h3 className='text-sm font-medium'>{t('Protocol probe')}</h3>
                  <p className='text-muted-foreground text-sm'>
                    {t(
                      'Probe one representative upstream model. Chat is tried only when Responses is unavailable.'
                    )}
                  </p>
                </div>
                <div className='grid gap-2 sm:grid-cols-[minmax(0,1fr)_auto]'>
                  <NativeSelect
                    className='w-full'
                    value={props.probeModel}
                    onChange={(event) =>
                      props.onProbeModelChange(event.target.value)
                    }
                    aria-label={t('Representative model')}
                  >
                    {props.selectedModels.map((model) => (
                      <NativeSelectOption key={model} value={model}>
                        {model}
                      </NativeSelectOption>
                    ))}
                  </NativeSelect>
                  <Button
                    variant='outline'
                    className={channelSetupLayoutClasses.mobileAction}
                    disabled={
                      props.probing ||
                      !props.probeModel ||
                      props.acceptedKeyIndexes.length === 0
                    }
                    onClick={props.onProbe}
                  >
                    {props.probing && <Spinner />}
                    {props.probing ? t('Probing...') : t('Run probe')}
                  </Button>
                </div>
                <ProtocolProbeStatus
                  status={props.probeStatus}
                  model={props.probeDisplayModel}
                  keyIndex={props.probeKeyIndex}
                  routes={props.routes}
                />
              </section>
              <ChannelDiscoveryEndpoints
                routes={props.routes}
                onSetRoute={props.onSetRoute}
              />
              <Separator />
            </>
          )}

          <section className='grid gap-4 sm:grid-cols-2'>
            <Field>
              <FieldLabel htmlFor='discovery-operation'>
                {t('Action')}
              </FieldLabel>
              <NativeSelect
                id='discovery-operation'
                className='w-full'
                {...props.form.register('operation')}
              >
                <NativeSelectOption value='create'>
                  {t('Create Channel')}
                </NativeSelectOption>
                <NativeSelectOption
                  value='update'
                  disabled={props.connection.matches.length === 0}
                >
                  {t('Update matching channel')}
                </NativeSelectOption>
              </NativeSelect>
            </Field>
            <Field data-invalid={Boolean(props.form.formState.errors.baseUrl)}>
              <FieldLabel htmlFor='discovery-base-url'>
                {t('Base URL')}
              </FieldLabel>
              <Input
                id='discovery-base-url'
                disabled={operation === 'update' && !syncConfiguration}
                aria-invalid={Boolean(props.form.formState.errors.baseUrl)}
                {...props.form.register('baseUrl')}
              />
              <FieldError>
                {props.form.formState.errors.baseUrl?.message
                  ? t(props.form.formState.errors.baseUrl.message)
                  : null}
              </FieldError>
            </Field>

            {operation === 'update' ? (
              <>
                <Field>
                  <FieldLabel htmlFor='discovery-channel'>
                    {t('Channel')}
                  </FieldLabel>
                  <NativeSelect
                    id='discovery-channel'
                    className='w-full'
                    {...props.form.register('channelId', {
                      valueAsNumber: true,
                    })}
                  >
                    {props.connection.matches.map((match) => (
                      <NativeSelectOption key={match.id} value={match.id}>
                        {match.name} (#{match.id})
                      </NativeSelectOption>
                    ))}
                  </NativeSelect>
                </Field>
                <Field>
                  <FieldLabel htmlFor='discovery-key-mode'>
                    {t('API key mode')}
                  </FieldLabel>
                  <NativeSelect
                    id='discovery-key-mode'
                    className='w-full'
                    {...props.form.register('keyMode')}
                  >
                    <NativeSelectOption value='append'>
                      {t('Append keys')}
                    </NativeSelectOption>
                    <NativeSelectOption value='replace'>
                      {t('Replace keys')}
                    </NativeSelectOption>
                  </NativeSelect>
                </Field>
                <Field orientation='horizontal'>
                  <FieldLabel htmlFor='discovery-sync'>
                    {t('Sync models and endpoints')}
                  </FieldLabel>
                  <Switch
                    id='discovery-sync'
                    checked={syncConfiguration}
                    onCheckedChange={(checked) =>
                      props.form.setValue('syncConfiguration', checked)
                    }
                  />
                </Field>
                <Field orientation='horizontal'>
                  <FieldLabel htmlFor='discovery-reenable'>
                    {t('Re-enable channel')}
                  </FieldLabel>
                  <Switch
                    id='discovery-reenable'
                    checked={props.form.watch('reenable')}
                    onCheckedChange={(checked) =>
                      props.form.setValue('reenable', checked)
                    }
                  />
                </Field>
              </>
            ) : (
              <>
                <Field>
                  <FieldLabel htmlFor='discovery-name'>{t('Name')}</FieldLabel>
                  <Input id='discovery-name' {...props.form.register('name')} />
                </Field>
                <Field>
                  <FieldLabel htmlFor='discovery-tag'>{t('Tag')}</FieldLabel>
                  <Input id='discovery-tag' {...props.form.register('tag')} />
                </Field>
                <Field>
                  <FieldLabel htmlFor='discovery-priority'>
                    {t('Priority')}
                  </FieldLabel>
                  <Input
                    id='discovery-priority'
                    type='number'
                    {...props.form.register('priority', {
                      valueAsNumber: true,
                    })}
                  />
                </Field>
                <Field orientation='horizontal'>
                  <FieldLabel htmlFor='discovery-enabled'>
                    {t('Enable channel')}
                  </FieldLabel>
                  <Switch
                    id='discovery-enabled'
                    checked={props.form.watch('enabled')}
                    onCheckedChange={(checked) =>
                      props.form.setValue('enabled', checked)
                    }
                  />
                </Field>
                <Field className='sm:col-span-2'>
                  <FieldLabel>{t('Groups')}</FieldLabel>
                  <div
                    role='group'
                    aria-label={t('Groups')}
                    className='grid grid-cols-[repeat(auto-fill,minmax(min(100%,13rem),1fr))] gap-2'
                  >
                    {props.groupOptions.map((group) => (
                      <FieldLabel
                        key={group}
                        htmlFor={`discovery-group-${group}`}
                        className='min-w-0 cursor-pointer justify-start rounded-lg border px-3 py-2.5'
                      >
                        <Checkbox
                          id={`discovery-group-${group}`}
                          checked={props.groups.includes(group)}
                          onCheckedChange={(checked) =>
                            props.onGroupsChange(
                              checked
                                ? [...new Set([...props.groups, group])].sort()
                                : props.groups.filter((item) => item !== group)
                            )
                          }
                        />
                        <span className='min-w-0 break-all'>{group}</span>
                      </FieldLabel>
                    ))}
                  </div>
                </Field>
              </>
            )}
          </section>

          <section className='space-y-2'>
            <h3 className='text-sm font-medium'>{t('Usable API keys')}</h3>
            <div className='flex flex-wrap gap-2'>
              {props.connection.usable_key_indexes.map((keyIndex) => (
                <FieldLabel
                  key={keyIndex}
                  htmlFor={`discovery-key-${keyIndex}`}
                  className='min-h-11 cursor-pointer rounded-lg border px-3'
                >
                  <Checkbox
                    id={`discovery-key-${keyIndex}`}
                    checked={props.acceptedKeyIndexes.includes(keyIndex)}
                    onCheckedChange={(checked) =>
                      props.onAcceptedKeysChange(
                        checked
                          ? [
                              ...new Set([
                                ...props.acceptedKeyIndexes,
                                keyIndex,
                              ]),
                            ].sort()
                          : props.acceptedKeyIndexes.filter(
                              (item) => item !== keyIndex
                            )
                      )
                    }
                  />
                  {t('Key {{number}}', { number: keyIndex + 1 })}
                </FieldLabel>
              ))}
            </div>
            {props.connection.rejected_key_count > 0 && (
              <FieldDescription>
                {t('{{count}} keys could not access model discovery', {
                  count: props.connection.rejected_key_count,
                })}
              </FieldDescription>
            )}
          </section>

          {showConfiguration && (
            <>
              <Separator />
              <ChannelDiscoveryOptions
                connection={props.connection}
                selectedModels={props.selectedModels}
                mapping={props.mapping}
                onToggleModel={props.onToggleModel}
                onModelsChange={props.onModelsChange}
                onSetMapping={props.onSetMapping}
                onRemoveMapping={props.onRemoveMapping}
              />
            </>
          )}
        </CardContent>
      </Card>
    </fieldset>
  )
}
