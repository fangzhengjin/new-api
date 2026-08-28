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
import { ArrowDown01Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useTranslation } from 'react-i18next'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import { FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'

import type {
  ChannelNormalizationData,
  ChannelNormalizationItem,
  ChannelNormalizationSelection,
} from '../channel-setup'
import {
  countNormalizationChanges,
  createDefaultNormalizationSelection,
  createEmptyNormalizationSelection,
  createFullNormalizationSelection,
  reconcileNormalizationSelection,
} from '../lib/channel-normalization-selection'

type ChannelNormalizationPreviewProps = {
  data: ChannelNormalizationData
  selections: Record<number, ChannelNormalizationSelection>
  onSelectionChange: (
    channelId: number,
    selection: ChannelNormalizationSelection
  ) => void
  onResetSafeDefaults: () => void
}

function toggleValue(values: string[], value: string, checked: boolean) {
  if (checked) return [...new Set([...values, value])].sort()
  return values.filter((item) => item !== value)
}

/**
 * Renders per-channel diffs and keeps dependent selections consistent.
 * @param props - Scan data, current selections, and selection callbacks.
 * @returns The editable batch-normalization preview.
 */
export function ChannelNormalizationPreview(
  props: ChannelNormalizationPreviewProps
) {
  const { t } = useTranslation()
  const result = props.data.task.result
  const items = result?.items ?? []
  const failures = result?.failures ?? []

  const getMetadata = (item: ChannelNormalizationItem) =>
    props.data.channels[String(item.channel_id)] ?? {
      models: [],
      model_mapping: {},
    }

  const update = (
    item: ChannelNormalizationItem,
    next: ChannelNormalizationSelection
  ) => {
    props.onSelectionChange(
      item.channel_id,
      reconcileNormalizationSelection(item, getMetadata(item), next)
    )
  }

  const selectAllChanges = (item: ChannelNormalizationItem) => {
    props.onSelectionChange(
      item.channel_id,
      createFullNormalizationSelection(item, getMetadata(item))
    )
  }

  const clearSelection = (item: ChannelNormalizationItem) => {
    props.onSelectionChange(
      item.channel_id,
      createEmptyNormalizationSelection(item)
    )
  }

  return (
    <div className='space-y-4'>
      <div className='flex flex-col gap-3 sm:flex-row sm:items-end sm:justify-between'>
        <div>
          <h2 className='text-base font-semibold'>
            {t('Normalization preview')}
          </h2>
          <p className='text-muted-foreground mt-1 text-sm'>
            {t(
              'Review each model and mapping adjustment. Nothing has been applied yet.'
            )}
          </p>
          <p className='text-muted-foreground mt-1 text-xs'>
            {t(
              'Safe defaults select additions and new mappings. Removals and existing mapping changes stay unselected.'
            )}
          </p>
        </div>
        {items.length > 0 ? (
          <div className='flex flex-wrap gap-2'>
            <Button
              variant='outline'
              onClick={() => {
                for (const item of items) selectAllChanges(item)
              }}
            >
              {t('Select all')}
            </Button>
            <Button
              variant='outline'
              onClick={() => {
                for (const item of items) clearSelection(item)
              }}
            >
              {t('Clear selection')}
            </Button>
            <Button variant='outline' onClick={props.onResetSafeDefaults}>
              {t('Restore safe defaults')}
            </Button>
          </div>
        ) : null}
      </div>

      {result && (
        <dl className='bg-border grid grid-cols-2 gap-px overflow-hidden rounded-xl border sm:grid-cols-4'>
          {[
            [t('Scanned'), result.summary.scanned],
            [t('Changed'), result.summary.changed],
            [t('Skipped'), result.summary.skipped],
            [t('Failed'), result.summary.failed],
          ].map(([label, value]) => (
            <div key={String(label)} className='bg-card p-3'>
              <dt className='text-muted-foreground text-xs'>{label}</dt>
              <dd className='mt-1 text-lg font-semibold'>{value}</dd>
            </div>
          ))}
        </dl>
      )}

      {failures.length > 0 && (
        <Alert variant='destructive'>
          <AlertTitle>
            {t('{{count}} channels could not be fully scanned', {
              count: failures.length,
            })}
          </AlertTitle>
          <AlertDescription>
            <ul className='mt-1 space-y-1'>
              {failures.map((failure) => (
                <li key={`${failure.channel_id}-${failure.error_message}`}>
                  #{failure.channel_id}{' '}
                  {props.data.channels[String(failure.channel_id)]?.name ?? ''}:{' '}
                  {failure.error_message}
                </li>
              ))}
            </ul>
          </AlertDescription>
        </Alert>
      )}

      {items.length === 0 ? (
        <Card size='sm'>
          <CardHeader>
            <CardTitle>{t('No changes')}</CardTitle>
            {failures.length === 0 ? (
              <CardDescription>
                {t(
                  'All scanned channels already follow the normalization rules.'
                )}
              </CardDescription>
            ) : null}
          </CardHeader>
        </Card>
      ) : (
        <div className='space-y-3'>
          {items.map((item) => {
            const metadata = props.data.channels[String(item.channel_id)] ?? {
              name: `#${item.channel_id}`,
              models: [],
              model_mapping: {},
            }
            const selection =
              props.selections[item.channel_id] ??
              createDefaultNormalizationSelection(item, metadata)
            const mappingWarnings = Object.entries(item.mapping_warnings ?? {})
            const modelChangesSelected =
              selection.add_models.length > 0 ||
              selection.remove_models.length > 0
            const projectedModels = new Set(metadata.models)
            for (const model of selection.add_models) projectedModels.add(model)
            for (const model of selection.remove_models) {
              projectedModels.delete(model)
            }
            const selectedCount = countNormalizationChanges(selection)
            return (
              <Collapsible
                key={item.channel_id}
                defaultOpen
                render={<Card size='sm' />}
              >
                <CardHeader>
                  <CardTitle className='flex flex-wrap items-center gap-2'>
                    <span>{metadata.name}</span>
                    <Badge variant='outline'>#{item.channel_id}</Badge>
                    <Badge
                      variant={selectedCount > 0 ? 'secondary' : 'outline'}
                    >
                      {t('Selected changes: {{count}}', {
                        count: selectedCount,
                      })}
                    </Badge>
                    {mappingWarnings.length > 0 && (
                      <Badge variant='warning'>
                        {t('Warnings: {{count}}', {
                          count: mappingWarnings.length,
                        })}
                      </Badge>
                    )}
                  </CardTitle>
                  <CardDescription>
                    {t('Models: {{before}} → {{after}}', {
                      before: metadata.models.length,
                      after: projectedModels.size,
                    })}
                  </CardDescription>
                  <CardAction>
                    <CollapsibleTrigger
                      render={
                        <Button
                          variant='ghost'
                          size='icon-sm'
                          aria-label={`${metadata.name}: ${t('Details')}`}
                        />
                      }
                      className='group'
                    >
                      <HugeiconsIcon
                        icon={ArrowDown01Icon}
                        strokeWidth={2}
                        aria-hidden='true'
                        className='transition-transform group-data-[panel-open]:rotate-180'
                      />
                    </CollapsibleTrigger>
                  </CardAction>
                </CardHeader>
                <CollapsibleContent
                  render={<CardContent className='space-y-4' />}
                >
                  <div className='flex flex-wrap gap-2'>
                    <Button
                      variant='outline'
                      size='sm'
                      onClick={() => selectAllChanges(item)}
                    >
                      {t('Select all')}
                    </Button>
                    <Button
                      variant='outline'
                      size='sm'
                      onClick={() => clearSelection(item)}
                    >
                      {t('Clear selection')}
                    </Button>
                  </div>
                  {mappingWarnings.length > 0 ? (
                    <Alert>
                      <AlertTitle>
                        {t(
                          'Some retained mapping targets were not returned by the upstream model list.'
                        )}
                      </AlertTitle>
                      <AlertDescription>
                        <ul className='mt-1 space-y-1'>
                          {mappingWarnings.map(([source, target]) => (
                            <li key={source} className='break-all'>
                              {source} → {target}
                            </li>
                          ))}
                        </ul>
                      </AlertDescription>
                    </Alert>
                  ) : null}
                  {(item.add_models?.length ?? 0) > 0 && (
                    <section className='space-y-2'>
                      <h3 className='text-sm font-medium'>{t('Add Models')}</h3>
                      <div className='grid gap-1.5 sm:grid-cols-2 xl:grid-cols-3'>
                        {item.add_models?.map((model) => (
                          <FieldLabel
                            key={model}
                            htmlFor={`normalize-${item.channel_id}-add-${model}`}
                            className='min-h-11 cursor-pointer rounded-lg border px-3'
                          >
                            <Checkbox
                              id={`normalize-${item.channel_id}-add-${model}`}
                              checked={selection.add_models.includes(model)}
                              onCheckedChange={(checked) =>
                                update(item, {
                                  ...selection,
                                  add_models: toggleValue(
                                    selection.add_models,
                                    model,
                                    checked === true
                                  ),
                                })
                              }
                            />
                            <span className='break-all'>{model}</span>
                          </FieldLabel>
                        ))}
                      </div>
                    </section>
                  )}

                  {(item.remove_models?.length ?? 0) > 0 && (
                    <section className='space-y-2'>
                      <h3 className='text-sm font-medium'>
                        {t('Remove Models')}
                      </h3>
                      <div className='grid gap-1.5 sm:grid-cols-2 xl:grid-cols-3'>
                        {item.remove_models?.map((model) => (
                          <FieldLabel
                            key={model}
                            htmlFor={`normalize-${item.channel_id}-remove-${model}`}
                            className='min-h-11 cursor-pointer rounded-lg border px-3'
                          >
                            <Checkbox
                              id={`normalize-${item.channel_id}-remove-${model}`}
                              checked={selection.remove_models.includes(model)}
                              onCheckedChange={(checked) =>
                                update(item, {
                                  ...selection,
                                  remove_models: toggleValue(
                                    selection.remove_models,
                                    model,
                                    checked === true
                                  ),
                                })
                              }
                            />
                            <span className='break-all'>{model}</span>
                          </FieldLabel>
                        ))}
                      </div>
                    </section>
                  )}

                  {Object.keys(item.mapping_set ?? {}).length > 0 && (
                    <section className='space-y-2'>
                      <h3 className='text-sm font-medium'>
                        {t('Set model mappings')}
                      </h3>
                      <div className='space-y-1.5'>
                        {Object.entries(item.mapping_set ?? {}).map(
                          ([source, suggestedTarget]) => {
                            const selectedTarget = selection.mapping_set[source]
                            const overwrites =
                              metadata.model_mapping[source] !== undefined
                            return (
                              <div
                                key={source}
                                className='grid gap-2 rounded-lg border p-2 sm:grid-cols-[auto_minmax(0,1fr)_minmax(0,1fr)] sm:items-center'
                              >
                                <Checkbox
                                  checked={selectedTarget !== undefined}
                                  aria-label={t('Apply mapping for {{model}}', {
                                    model: source,
                                  })}
                                  onCheckedChange={(checked) => {
                                    const mappingSet = {
                                      ...selection.mapping_set,
                                    }
                                    let addModels = selection.add_models
                                    if (checked) {
                                      mappingSet[source] = suggestedTarget
                                      if (item.add_models?.includes(source)) {
                                        addModels = toggleValue(
                                          addModels,
                                          source,
                                          true
                                        )
                                      }
                                    } else {
                                      delete mappingSet[source]
                                      if (item.add_models?.includes(source)) {
                                        addModels = addModels.filter(
                                          (model) => model !== source
                                        )
                                      }
                                    }
                                    update(item, {
                                      ...selection,
                                      add_models: addModels,
                                      mapping_set: mappingSet,
                                    })
                                  }}
                                />
                                <span className='min-w-0 text-sm font-medium break-all'>
                                  {source}
                                  {overwrites && (
                                    <Badge variant='warning' className='ml-2'>
                                      {t('Overwrite')}
                                    </Badge>
                                  )}
                                </span>
                                <Input
                                  value={selectedTarget ?? suggestedTarget}
                                  disabled={selectedTarget === undefined}
                                  aria-invalid={
                                    selectedTarget !== undefined &&
                                    selectedTarget.trim() === ''
                                  }
                                  aria-label={t(
                                    'Mapping target for {{model}}',
                                    {
                                      model: source,
                                    }
                                  )}
                                  onChange={(event) =>
                                    update(item, {
                                      ...selection,
                                      mapping_set: {
                                        ...selection.mapping_set,
                                        [source]: event.target.value,
                                      },
                                    })
                                  }
                                />
                              </div>
                            )
                          }
                        )}
                      </div>
                    </section>
                  )}

                  {(item.mapping_remove?.length ?? 0) > 0 && (
                    <section className='space-y-2'>
                      <h3 className='text-sm font-medium'>
                        {t('Remove model mappings')}
                      </h3>
                      <div className='grid gap-1.5 sm:grid-cols-2 xl:grid-cols-3'>
                        {item.mapping_remove?.map((source) => (
                          <FieldLabel
                            key={source}
                            htmlFor={`normalize-${item.channel_id}-mapping-remove-${source}`}
                            className='min-h-11 cursor-pointer rounded-lg border px-3'
                          >
                            <Checkbox
                              id={`normalize-${item.channel_id}-mapping-remove-${source}`}
                              checked={selection.mapping_remove.includes(
                                source
                              )}
                              onCheckedChange={(checked) => {
                                const removing = checked === true
                                update(item, {
                                  ...selection,
                                  mapping_remove: toggleValue(
                                    selection.mapping_remove,
                                    source,
                                    removing
                                  ),
                                  remove_models: removing
                                    ? selection.remove_models
                                    : selection.remove_models.filter(
                                        (model) => model !== source
                                      ),
                                })
                              }}
                            />
                            <span className='break-all'>{source}</span>
                          </FieldLabel>
                        ))}
                      </div>
                    </section>
                  )}

                  {(item.sort_changed || modelChangesSelected) && (
                    <div className='flex min-h-11 items-center justify-between gap-3 rounded-lg border px-3 py-2'>
                      <div>
                        {modelChangesSelected ? (
                          <p className='text-sm font-medium'>
                            {t('Sort model names')}
                          </p>
                        ) : (
                          <FieldLabel
                            htmlFor={`normalize-${item.channel_id}-sort`}
                          >
                            {t('Sort model names')}
                          </FieldLabel>
                        )}
                        <p className='text-muted-foreground text-xs'>
                          {modelChangesSelected
                            ? t(
                                'Model names are sorted automatically when models change.'
                              )
                            : t('Keeps the stored model list deterministic.')}
                        </p>
                      </div>
                      {!modelChangesSelected && (
                        <Switch
                          id={`normalize-${item.channel_id}-sort`}
                          checked={selection.sort_models}
                          onCheckedChange={(checked) =>
                            update(item, {
                              ...selection,
                              sort_models: checked,
                            })
                          }
                        />
                      )}
                    </div>
                  )}
                </CollapsibleContent>
              </Collapsible>
            )
          })}
        </div>
      )}
    </div>
  )
}
