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
import { Add01Icon, Delete02Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { FieldDescription, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { Switch } from '@/components/ui/switch'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'

import type {
  ChannelDiscoveryChoice,
  ChannelDiscoveryConnection,
} from '../channel-setup'

const routeDefaults: Record<string, string> = {
  models: '/v1/models',
  responses: '/v1/responses',
  compact: '/v1/responses/compact',
  chat: '/v1/chat/completions',
  messages: '/v1/messages',
  image_generations: '/v1/images/generations',
  image_edits: '/v1/images/edits',
}

type ChannelDiscoveryOptionsProps = {
  connection: ChannelDiscoveryConnection
  selectedModels: string[]
  mapping: Record<string, string>
  onToggleModel: (choice: ChannelDiscoveryChoice, checked: boolean) => void
  onModelsChange: (choices: ChannelDiscoveryChoice[]) => void
  onSetMapping: (source: string, target: string) => void
  onRemoveMapping: (source: string) => void
}

type ChannelDiscoveryEndpointsProps = {
  routes: Record<string, string>
  onSetRoute: (protocol: string, path: string | null) => void
}

/**
 * Renders editable model and mapping choices for one connection.
 * @param props - Current choices plus callbacks that update the parent draft.
 * @returns The model configuration controls.
 */
export function ChannelDiscoveryOptions(props: ChannelDiscoveryOptionsProps) {
  const { t } = useTranslation()
  const [newMappingSource, setNewMappingSource] = useState('')
  const [newMappingTarget, setNewMappingTarget] = useState(
    props.connection.models[0] ?? ''
  )
  const effectiveTarget = props.connection.models.includes(newMappingTarget)
    ? newMappingTarget
    : (props.connection.models[0] ?? '')

  const selectedChoices = new Map<string, ChannelDiscoveryChoice>()
  for (const choice of props.connection.choices) {
    if (!selectedChoices.has(choice.model)) {
      selectedChoices.set(choice.model, choice)
    }
  }
  const visibleChoices = [...selectedChoices.values()]
  const selectedModelSet = new Set(props.selectedModels)
  const hiddenSelectedChoices = props.selectedModels
    .filter((model) => !selectedChoices.has(model))
    .map((model) => ({
      source: model,
      model,
      target: props.mapping[model] ?? model,
      recommended: false,
    }))
  const selectedVisibleCount = visibleChoices.filter((choice) =>
    selectedModelSet.has(choice.model)
  ).length
  const allVisibleSelected =
    visibleChoices.length > 0 && selectedVisibleCount === visibleChoices.length

  const addMapping = () => {
    const source = newMappingSource.trim()
    const target = effectiveTarget.trim()
    if (!source || !target) {
      return
    }
    props.onSetMapping(source, target)
    setNewMappingSource('')
  }

  return (
    <Tabs defaultValue='models' className='min-w-0'>
      <TabsList className='grid w-full grid-cols-2 sm:w-fit'>
        <TabsTrigger value='models'>{t('Models')}</TabsTrigger>
        <TabsTrigger value='mapping'>{t('Mapping')}</TabsTrigger>
      </TabsList>

      <TabsContent value='models' className='space-y-2'>
        <div className='flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between'>
          <p className='text-muted-foreground text-sm'>
            {t('{{selected}} of {{total}} selected', {
              selected: selectedVisibleCount,
              total: visibleChoices.length,
            })}
          </p>
          <div className='flex flex-wrap gap-2'>
            <Button
              type='button'
              variant='outline'
              size='sm'
              disabled={visibleChoices.length === 0 || allVisibleSelected}
              onClick={() =>
                props.onModelsChange([
                  ...hiddenSelectedChoices,
                  ...visibleChoices,
                ])
              }
            >
              {t('Select all')}
            </Button>
            <Button
              type='button'
              variant='outline'
              size='sm'
              disabled={visibleChoices.length === 0}
              onClick={() =>
                props.onModelsChange([
                  ...hiddenSelectedChoices,
                  ...visibleChoices.filter(
                    (choice) => !selectedModelSet.has(choice.model)
                  ),
                ])
              }
            >
              {t('Invert selection')}
            </Button>
            <Button
              type='button'
              variant='outline'
              size='sm'
              disabled={props.selectedModels.length === 0}
              onClick={() => props.onModelsChange([])}
            >
              {t('Clear selection')}
            </Button>
          </div>
        </div>
        <div className='max-h-[28rem] space-y-1.5 overflow-y-auto rounded-lg border p-2'>
          {visibleChoices.map((choice) => {
            const checked = props.selectedModels.includes(choice.model)
            return (
              <div
                key={choice.model}
                className='hover:bg-muted/60 flex min-h-11 items-start gap-2 rounded-lg px-2 py-2'
              >
                <Checkbox
                  id={`discovery-model-${choice.model}`}
                  checked={checked}
                  onCheckedChange={(next) =>
                    props.onToggleModel(choice, next === true)
                  }
                  className='mt-0.5'
                />
                <div className='min-w-0 flex-1'>
                  <FieldLabel
                    htmlFor={`discovery-model-${choice.model}`}
                    className='w-full cursor-pointer justify-between gap-2'
                  >
                    <span className='break-all'>{choice.model}</span>
                    <span className='flex shrink-0 gap-1'>
                      {choice.recommended && (
                        <Badge variant='secondary'>{t('Recommended')}</Badge>
                      )}
                    </span>
                  </FieldLabel>
                  {choice.model !== choice.source && (
                    <p className='text-muted-foreground mt-1 text-xs break-all'>
                      {t('Upstream')}: {choice.source}
                    </p>
                  )}
                </div>
              </div>
            )
          })}
        </div>
      </TabsContent>

      <TabsContent value='mapping' className='space-y-3'>
        <FieldDescription>
          {t(
            'Mapping sources are exposed model names; targets are upstream model IDs.'
          )}
        </FieldDescription>
        <div className='space-y-2 rounded-lg border p-2'>
          {Object.keys(props.mapping).length === 0 && (
            <p className='text-muted-foreground px-1 py-2 text-sm'>
              {t('No model mappings')}
            </p>
          )}
          {Object.entries(props.mapping).map(([source, target]) => (
            <div
              key={source}
              className='bg-muted/50 grid gap-2 rounded-lg p-2 sm:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_auto] sm:items-center'
            >
              <div className='min-w-0 text-sm font-medium break-all'>
                {source}
              </div>
              <NativeSelect
                className='w-full'
                aria-label={t('Upstream model for {{model}}', {
                  model: source,
                })}
                value={target}
                onChange={(event) =>
                  props.onSetMapping(source, event.target.value)
                }
              >
                {props.connection.models.map((model) => (
                  <NativeSelectOption key={model} value={model}>
                    {model}
                  </NativeSelectOption>
                ))}
              </NativeSelect>
              <Button
                type='button'
                variant='ghost'
                size='icon'
                className='max-sm:min-h-11 max-sm:w-full'
                aria-label={t('Remove mapping for {{model}}', {
                  model: source,
                })}
                onClick={() => props.onRemoveMapping(source)}
              >
                <HugeiconsIcon icon={Delete02Icon} strokeWidth={2} />
              </Button>
            </div>
          ))}
        </div>
        <div className='grid gap-2 sm:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_auto]'>
          <Input
            value={newMappingSource}
            onChange={(event) => setNewMappingSource(event.target.value)}
            placeholder={t('Exposed model name')}
            aria-label={t('Exposed model name')}
          />
          <NativeSelect
            className='w-full'
            value={effectiveTarget}
            onChange={(event) => setNewMappingTarget(event.target.value)}
            aria-label={t('Upstream model')}
          >
            {props.connection.models.map((model) => (
              <NativeSelectOption key={model} value={model}>
                {model}
              </NativeSelectOption>
            ))}
          </NativeSelect>
          <Button
            type='button'
            variant='outline'
            onClick={addMapping}
            disabled={!newMappingSource.trim() || !effectiveTarget}
            className='max-sm:min-h-11'
          >
            <HugeiconsIcon icon={Add01Icon} strokeWidth={2} />
            {t('Add Mapping')}
          </Button>
        </div>
      </TabsContent>
    </Tabs>
  )
}

/** Renders detected protocol endpoints beside the probe that produced them. */
export function ChannelDiscoveryEndpoints(
  props: ChannelDiscoveryEndpointsProps
) {
  const { t } = useTranslation()

  return (
    <section className='space-y-2'>
      <h3 className='text-sm font-medium'>{t('Endpoints')}</h3>
      <div className='space-y-2 rounded-lg border p-2'>
        {Object.keys(routeDefaults).map((protocol) => {
          const enabled = props.routes[protocol] !== undefined
          return (
            <div
              key={protocol}
              className='grid min-h-11 gap-2 rounded-lg px-2 py-2 sm:grid-cols-[7rem_auto_minmax(0,1fr)] sm:items-center'
            >
              <FieldLabel htmlFor={`route-${protocol}`} className='capitalize'>
                {protocol.replaceAll('_', ' ')}
              </FieldLabel>
              <Switch
                id={`route-${protocol}`}
                checked={enabled}
                onCheckedChange={(checked) =>
                  props.onSetRoute(
                    protocol,
                    checked ? routeDefaults[protocol] : null
                  )
                }
              />
              <Input
                value={props.routes[protocol] ?? routeDefaults[protocol]}
                disabled={!enabled}
                onChange={(event) =>
                  props.onSetRoute(protocol, event.target.value)
                }
                aria-label={t('{{protocol}} upstream path', { protocol })}
              />
            </div>
          )
        })}
      </div>
    </section>
  )
}
