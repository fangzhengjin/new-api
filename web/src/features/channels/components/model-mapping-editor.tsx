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
import { Code, Plus, Table, Trash2 } from 'lucide-react'
import { useEffect, useId, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { JsonCodeEditor } from '@/components/json-code-editor'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'

type ModelMappingEditorProps = {
  value: string
  onChange: (value: string) => void
  disabled?: boolean
  sourceModelOptions?: string[]
  targetModelOptions?: string[]
  userHiddenModelMappings?: string[]
  onUserHiddenModelMappingsChange?: (models: string[]) => void
  userHiddenModelMappingsDisabled?: boolean
}

type MappingRow = {
  id: string
  from: string
  to: string
}

const DUPLICATE_MAPPING_SENTINEL = '{ "duplicate_source_models": '

function getDuplicateSources(rows: MappingRow[]): string[] {
  const seen = new Set<string>()
  const duplicates = new Set<string>()

  for (const row of rows) {
    const source = row.from.trim()
    if (!source) continue
    if (seen.has(source)) {
      duplicates.add(source)
    } else {
      seen.add(source)
    }
  }

  return [...duplicates]
}

export function ModelMappingEditor(props: ModelMappingEditorProps) {
  const { t } = useTranslation()
  const sourceListId = useId()
  const targetListId = useId()
  const [mode, setMode] = useState<'visual' | 'json'>('visual')
  const [rows, setRows] = useState<MappingRow[]>([])
  const [jsonValue, setJsonValue] = useState(props.value)
  const [jsonError, setJsonError] = useState<string | null>(null)
  const nextRowIdRef = useRef(0)
  const duplicateSources = useMemo(() => getDuplicateSources(rows), [rows])
  const showUserVisibility = props.onUserHiddenModelMappingsChange != null

  const createRowId = () => {
    nextRowIdRef.current += 1
    return `mapping-${nextRowIdRef.current}`
  }

  const reconcileHiddenMappings = (sourceModels: string[]) => {
    if (
      !props.onUserHiddenModelMappingsChange ||
      props.userHiddenModelMappingsDisabled
    ) {
      return
    }
    const sources = new Set(
      sourceModels.map((model) => model.trim()).filter(Boolean)
    )
    const current = props.userHiddenModelMappings || []
    const next = current.filter((model) => sources.has(model))
    if (
      next.length === current.length &&
      next.every((model, index) => model === current[index])
    ) {
      return
    }
    props.onUserHiddenModelMappingsChange(next)
  }

  const parseJsonToRows = (json: string): boolean => {
    try {
      if (!json.trim()) {
        setRows([])
        setJsonError(null)
        reconcileHiddenMappings([])
        return true
      }
      const parsed = JSON.parse(json)
      if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
        setJsonError(t('Model mapping must be a valid JSON object'))
        return false
      }
      const entries = Object.entries(parsed)
      const invalidValue = entries.find(([, to]) => typeof to !== 'string')
      if (invalidValue) {
        setJsonError(t('Model mapping values must be strings'))
        return false
      }
      reconcileHiddenMappings(entries.map(([from]) => from))
      setRows((previousRows) => {
        const remainingRows = [...previousRows]
        return entries.map(([from, to], index) => {
          const toString = String(to)
          const existingIndex = remainingRows.findIndex(
            (row) =>
              row.from === from ||
              (row.from === from && row.to === toString) ||
              previousRows[index]?.id === row.id
          )
          if (existingIndex >= 0) {
            const [existing] = remainingRows.splice(existingIndex, 1)
            return {
              id: existing.id,
              from,
              to: toString,
            }
          }
          return {
            id: createRowId(),
            from,
            to: toString,
          }
        })
      })
      setJsonError(null)
      return true
    } catch {
      setJsonError(t('Model mapping must be valid JSON format'))
      return false
    }
  }

  // Parse JSON to rows when value changes externally
  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setJsonValue(props.value)
    parseJsonToRows(props.value)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [props.value])

  const convertRowsToJson = (updatedRows: MappingRow[]): string => {
    if (updatedRows.length === 0) {
      return ''
    }
    const obj: Record<string, string> = {}
    updatedRows.forEach((row) => {
      if (row.from.trim()) {
        obj[row.from.trim()] = row.to.trim()
      }
    })
    return JSON.stringify(obj, null, 2)
  }

  const syncRows = (updatedRows: MappingRow[]) => {
    setRows(updatedRows)
    const duplicates = getDuplicateSources(updatedRows)
    if (duplicates.length > 0) {
      setJsonError(t('Duplicate source model mappings are not allowed'))
      setJsonValue(DUPLICATE_MAPPING_SENTINEL)
      props.onChange(DUPLICATE_MAPPING_SENTINEL)
      return
    }

    reconcileHiddenMappings(updatedRows.map((row) => row.from))
    const json = convertRowsToJson(updatedRows)
    setJsonError(null)
    setJsonValue(json)
    props.onChange(json)
  }

  const handleAddRow = () => {
    const newRow: MappingRow = {
      id: createRowId(),
      from: '',
      to: '',
    }
    syncRows([...rows, newRow])
  }

  const handleVisibilityChange = (source: string, visible: boolean) => {
    const model = source.trim()
    if (!model || props.userHiddenModelMappingsDisabled) return
    const hiddenModels = new Set(props.userHiddenModelMappings || [])
    if (visible) {
      hiddenModels.delete(model)
    } else {
      hiddenModels.add(model)
    }
    props.onUserHiddenModelMappingsChange?.([...hiddenModels])
  }

  const handleDeleteRow = (id: string) => {
    syncRows(rows.filter((row) => row.id !== id))
  }

  const handleRowChange = (
    id: string,
    field: 'from' | 'to',
    newValue: string
  ) => {
    const updatedRows = rows.map((row) =>
      row.id === id ? { ...row, [field]: newValue } : row
    )
    syncRows(updatedRows)
  }

  const handleJsonChange = (newJson: string) => {
    setJsonValue(newJson)
    props.onChange(newJson)
    parseJsonToRows(newJson)
  }

  const handleFillTemplate = () => {
    const template = JSON.stringify(
      { 'gpt-3.5-turbo': 'gpt-3.5-turbo-0125' },
      null,
      2
    )
    setJsonValue(template)
    props.onChange(template)
    parseJsonToRows(template)
  }

  const handleModeChange = (nextMode: string) => {
    if (nextMode !== 'visual' && nextMode !== 'json') return
    if (nextMode === 'json') {
      const duplicates = getDuplicateSources(rows)
      if (duplicates.length === 0) {
        const json = convertRowsToJson(rows)
        setJsonValue(json)
        props.onChange(json)
      }
      setMode('json')
      return
    }
    parseJsonToRows(jsonValue)
    setMode('visual')
  }

  return (
    <div className='space-y-2'>
      <Tabs value={mode} onValueChange={handleModeChange} className='space-y-2'>
        <div className='flex items-center justify-between gap-3'>
          <TabsList>
            <TabsTrigger value='visual'>
              <Table className='h-4 w-4' aria-hidden='true' />
              {t('Visual')}
            </TabsTrigger>
            <TabsTrigger value='json'>
              <Code className='h-4 w-4' aria-hidden='true' />
              {t('JSON')}
            </TabsTrigger>
          </TabsList>
          <Button
            type='button'
            variant='link'
            size='sm'
            className='h-auto p-0'
            onClick={handleFillTemplate}
            disabled={props.disabled}
          >
            {t('Fill Template')}
          </Button>
        </div>

        {jsonError && (
          <Alert variant='destructive'>
            <AlertDescription>{jsonError}</AlertDescription>
          </Alert>
        )}

        {duplicateSources.length > 0 && (
          <Alert>
            <AlertDescription>
              {t('Duplicate source model(s): {{models}}', {
                models: duplicateSources.join(', '),
              })}
            </AlertDescription>
          </Alert>
        )}

        <TabsContent value='visual' className='space-y-2'>
          {rows.length > 0 ? (
            <div className='space-y-2'>
              <div
                className={
                  showUserVisibility
                    ? 'grid grid-cols-[minmax(0,1fr)_minmax(0,1fr)_6rem_2.5rem] gap-2 text-sm font-medium'
                    : 'grid grid-cols-[1fr_1fr_auto] gap-2 text-sm font-medium'
                }
              >
                <div>{t('Original Model')}</div>
                <div>{t('Replacement Model')}</div>
                {showUserVisibility && (
                  <div className='text-center'>{t('User Visible')}</div>
                )}
                <div className='w-10' />
              </div>
              {rows.map((row) => (
                <div
                  key={row.id}
                  className={
                    showUserVisibility
                      ? 'grid grid-cols-[minmax(0,1fr)_minmax(0,1fr)_6rem_2.5rem] gap-2'
                      : 'grid grid-cols-[1fr_1fr_auto] gap-2'
                  }
                >
                  <Input
                    value={row.from}
                    onChange={(e) =>
                      handleRowChange(row.id, 'from', e.target.value)
                    }
                    placeholder='gpt-3.5-turbo'
                    disabled={props.disabled}
                    list={sourceListId}
                  />
                  <Input
                    value={row.to}
                    onChange={(e) =>
                      handleRowChange(row.id, 'to', e.target.value)
                    }
                    placeholder='gpt-3.5-turbo-0125'
                    disabled={props.disabled}
                    list={targetListId}
                  />
                  {showUserVisibility && (
                    <div className='flex h-10 items-center justify-center'>
                      <Checkbox
                        checked={
                          !props.userHiddenModelMappings?.includes(
                            row.from.trim()
                          )
                        }
                        onCheckedChange={(checked) =>
                          handleVisibilityChange(row.from, checked)
                        }
                        disabled={
                          props.disabled ||
                          props.userHiddenModelMappingsDisabled ||
                          !row.from.trim()
                        }
                        aria-label={`${t('User Visible')}: ${row.from || t('Original Model')}`}
                      />
                    </div>
                  )}
                  <Button
                    type='button'
                    variant='ghost'
                    size='icon'
                    onClick={() => handleDeleteRow(row.id)}
                    disabled={props.disabled}
                    className='h-10 w-10'
                    aria-label={t('Delete mapping')}
                  >
                    <Trash2 className='h-4 w-4' aria-hidden='true' />
                  </Button>
                </div>
              ))}
            </div>
          ) : (
            <div className='text-muted-foreground flex h-24 items-center justify-center rounded-md border border-dashed text-sm'>
              {t(
                'No model mappings configured. Click "Add Mapping" to get started.'
              )}
            </div>
          )}
          <Button
            type='button'
            variant='outline'
            size='sm'
            onClick={handleAddRow}
            disabled={props.disabled}
            className='w-full'
          >
            <Plus className='mr-2 h-4 w-4' />
            {t('Add Mapping')}
          </Button>
        </TabsContent>
        <TabsContent value='json'>
          <JsonCodeEditor
            value={jsonValue}
            onChange={handleJsonChange}
            placeholder={t('{"original-model": "replacement-model"}')}
            disabled={props.disabled}
            className={jsonError ? 'border-destructive' : undefined}
            aria-invalid={Boolean(jsonError)}
            ariaLabel={t('Model Mapping')}
          />
        </TabsContent>
      </Tabs>

      {props.sourceModelOptions && props.sourceModelOptions.length > 0 && (
        <datalist id={sourceListId}>
          {props.sourceModelOptions.map((model) => (
            <option key={model} value={model} />
          ))}
        </datalist>
      )}
      {props.targetModelOptions && props.targetModelOptions.length > 0 && (
        <datalist id={targetListId}>
          {props.targetModelOptions.map((model) => (
            <option key={model} value={model} />
          ))}
        </datalist>
      )}
    </div>
  )
}
