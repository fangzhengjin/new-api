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
import { zodResolver } from '@hookform/resolvers/zod'
import { Code2, Eye, Plus, Trash2 } from 'lucide-react'
import { useMemo, useRef, useState } from 'react'
import { Controller, type Resolver } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { z } from 'zod'

import { StaticDataTable } from '@/components/data-table'
import { JsonCodeEditor } from '@/components/json-code-editor'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'

import { FormDirtyIndicator } from '../components/form-dirty-indicator'
import { FormNavigationGuard } from '../components/form-navigation-guard'
import {
  SettingsForm,
  SettingsFormGrid,
  SettingsFormSection,
} from '../components/settings-form-layout'
import { SettingsPage } from '../components/settings-page'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useSettingsForm } from '../hooks/use-settings-form'
import { useUpdateOption } from '../hooks/use-update-option'

type CycleQuotaSettings = {
  'quota_setting.settlement_lead_minutes': number
  'quota_setting.settlement_prompt': string
  'quota_setting.temporary_quota_projects': string
}

type FormValues = {
  quota_setting: {
    settlement_lead_minutes: number
    settlement_prompt: string
    temporary_quota_projects: string
  }
}

type ProjectRow = { id: number; name: string; enabled: boolean }
type EditorMode = 'visual' | 'json'

const defaultCycleQuotaSettings: CycleQuotaSettings = {
  'quota_setting.settlement_lead_minutes': 10,
  'quota_setting.settlement_prompt':
    '本期额度正在结算，暂时无法发起新请求，请稍后重试',
  'quota_setting.temporary_quota_projects': '{}',
}

function parseProjects(value: string): Record<string, boolean> | null {
  try {
    const parsed = JSON.parse(value) as unknown
    if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
      return null
    }
    for (const [name, enabled] of Object.entries(parsed)) {
      if (
        typeof enabled !== 'boolean' ||
        name !== name.trim() ||
        name.length === 0 ||
        [...name].length > 100
      ) {
        return null
      }
    }
    return Object.keys(parsed).length <= 100
      ? (parsed as Record<string, boolean>)
      : null
  } catch {
    return null
  }
}

function projectsToRows(projects: Record<string, boolean>): ProjectRow[] {
  return Object.entries(projects).map(([name, enabled], index) => ({
    id: index + 1,
    name,
    enabled,
  }))
}

function TemporaryQuotaProjectsEditor(props: {
  value: string
  onChange: (value: string) => void
  disabled: boolean
  validationError?: string
}) {
  const { t } = useTranslation()
  const initialProjects = parseProjects(props.value) ?? {}
  const [mode, setMode] = useState<EditorMode>('visual')
  const [rows, setRows] = useState<ProjectRow[]>(() =>
    projectsToRows(initialProjects)
  )
  const [jsonText, setJsonText] = useState(() =>
    JSON.stringify(initialProjects, null, 2)
  )
  const [error, setError] = useState('')
  const nextId = useRef(rows.length + 1)
  const pendingFocusId = useRef<number | null>(null)

  const validateRows = (nextRows: ProjectRow[]) => {
    if (nextRows.length > 100) return t('Up to 100 projects are allowed')
    const names = new Set<string>()
    for (const row of nextRows) {
      if (!row.name) return t('Project name is required')
      if (row.name !== row.name.trim()) {
        return t('Project names cannot have leading or trailing spaces')
      }
      if ([...row.name].length > 100) {
        return t('Project name must be 100 characters or fewer')
      }
      if (names.has(row.name)) return t('Project names must be unique')
      names.add(row.name)
    }
    return ''
  }

  const syncRows = (nextRows: ProjectRow[]) => {
    setRows(nextRows)
    const nextError = validateRows(nextRows)
    setError(nextError)
    if (nextError) {
      props.onChange('')
      return
    }
    const projects = Object.fromEntries(
      nextRows.map((row) => [row.name, row.enabled])
    )
    const nextJson = JSON.stringify(projects, null, 2)
    setJsonText(nextJson)
    props.onChange(JSON.stringify(projects))
  }

  const handleJsonChange = (value: string) => {
    setJsonText(value)
    props.onChange(value)
    const projects = parseProjects(value)
    if (!projects) {
      setError(t('JSON must map project names to true or false'))
      return
    }
    const nextRows = projectsToRows(projects)
    setRows(nextRows)
    nextId.current = nextRows.length + 1
    setError('')
  }

  const addRow = () => {
    if (rows.some((row) => !row.name.trim())) return
    const row = { id: nextId.current, name: '', enabled: true }
    nextId.current += 1
    pendingFocusId.current = row.id
    setRows([...rows, row])
    setError('')
    props.onChange('')
  }

  const hasIncompleteProject = rows.some((row) => !row.name.trim())
  let visibleError = error
  if (!visibleError && props.validationError) {
    visibleError = hasIncompleteProject
      ? t('Project name is required')
      : props.validationError
  }

  return (
    <Card>
      <CardHeader className='border-b'>
        <div className='flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between'>
          <div>
            <CardTitle>{t('Temporary quota projects')}</CardTitle>
            <CardDescription>
              {t(
                'Maintain projects available for temporary quota requests, only projects marked Selectable appear in the request list'
              )}
            </CardDescription>
          </div>
          <div className='flex flex-wrap gap-2 sm:justify-end'>
            <Button
              type='button'
              size='sm'
              variant='outline'
              onClick={() => setMode(mode === 'visual' ? 'json' : 'visual')}
              disabled={
                props.disabled || Boolean(error) || hasIncompleteProject
              }
            >
              {mode === 'visual' ? (
                <Code2 data-icon='inline-start' />
              ) : (
                <Eye data-icon='inline-start' />
              )}
              {mode === 'visual' ? t('Switch to JSON') : t('Switch to Visual')}
            </Button>
            <Button
              type='button'
              size='sm'
              onClick={addRow}
              disabled={
                props.disabled ||
                mode === 'json' ||
                hasIncompleteProject ||
                rows.length >= 100
              }
            >
              <Plus data-icon='inline-start' />
              {t('Add project')}
            </Button>
          </div>
        </div>
      </CardHeader>
      <CardContent className='flex flex-col gap-3'>
        {mode === 'visual' ? (
          <StaticDataTable
            data={rows}
            getRowKey={(row) => row.id}
            emptyContent={t('No temporary quota projects')}
            emptyClassName='text-muted-foreground py-8'
            columns={[
              {
                id: 'name',
                header: t('Project name'),
                cell: (row) => (
                  <Input
                    ref={(node) => {
                      if (node && pendingFocusId.current === row.id) {
                        pendingFocusId.current = null
                        node.focus()
                      }
                    }}
                    value={row.name}
                    maxLength={100}
                    aria-label={t('Project name')}
                    aria-invalid={Boolean(visibleError && !row.name.trim())}
                    disabled={props.disabled}
                    onChange={(event) =>
                      syncRows(
                        rows.map((item) =>
                          item.id === row.id
                            ? { ...item, name: event.target.value }
                            : item
                        )
                      )
                    }
                  />
                ),
              },
              {
                id: 'enabled',
                header: t('Selectable'),
                className: 'w-24',
                cell: (row) => (
                  <Switch
                    checked={row.enabled}
                    aria-label={`${t('Selectable')}: ${row.name || t('Project name')}`}
                    disabled={props.disabled}
                    onCheckedChange={(enabled) =>
                      syncRows(
                        rows.map((item) =>
                          item.id === row.id ? { ...item, enabled } : item
                        )
                      )
                    }
                  />
                ),
              },
              {
                id: 'actions',
                header: <span className='sr-only'>{t('Actions')}</span>,
                className: 'w-14',
                cellClassName: 'text-right',
                cell: (row) => (
                  <Button
                    type='button'
                    size='icon'
                    variant='ghost'
                    aria-label={t('Delete project')}
                    disabled={props.disabled}
                    onClick={() =>
                      syncRows(rows.filter((item) => item.id !== row.id))
                    }
                  >
                    <Trash2 className='text-destructive' />
                  </Button>
                ),
              },
            ]}
          />
        ) : (
          <JsonCodeEditor
            value={jsonText}
            onChange={handleJsonChange}
            disabled={props.disabled}
            heightClassName='h-64 min-h-64 max-h-64'
            ariaLabel={t('Temporary quota projects JSON')}
            aria-invalid={Boolean(visibleError)}
          />
        )}
        {visibleError ? (
          <p role='alert' className='text-destructive text-sm'>
            {visibleError}
          </p>
        ) : null}
      </CardContent>
    </Card>
  )
}

function CycleQuotaSettingsForm(props: { defaultValues: FormValues }) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const schema = useMemo(
    () =>
      z.object({
        quota_setting: z.object({
          settlement_lead_minutes: z.coerce
            .number()
            .int()
            .min(3, t('Enter a value from 3 to 60 minutes'))
            .max(60, t('Enter a value from 3 to 60 minutes')),
          settlement_prompt: z
            .string()
            .trim()
            .min(1, t('Settlement message is required'))
            .max(200, t('Settlement message must be 200 characters or fewer')),
          temporary_quota_projects: z
            .string()
            .refine(
              (value) => parseProjects(value) !== null,
              t('JSON must map project names to true or false')
            ),
        }),
      }),
    [t]
  )
  const { form, handleSubmit, isDirty, isSubmitting } =
    useSettingsForm<FormValues>({
      resolver: zodResolver(schema) as Resolver<FormValues>,
      defaultValues: props.defaultValues,
      onSubmit: async (_data, changedFields) => {
        await updateOption.mutateAsync(
          Object.entries(changedFields).map(([key, value]) => ({
            key,
            value: value as string | number,
          }))
        )
      },
    })

  return (
    <SettingsSection title={t('Cycle Quota Management')} className='shrink-0'>
      <FormNavigationGuard when={isDirty} />
      <Form {...form}>
        <SettingsForm onSubmit={handleSubmit}>
          <SettingsPageFormActions
            onSave={handleSubmit}
            isSaving={updateOption.isPending || isSubmitting}
          />
          <FormDirtyIndicator isDirty={isDirty} />

          <SettingsFormSection
            title={t('Quota cycle settlement')}
            description={t(
              'Stop new requests before the cycle ends, then handle balances and close the cycle after in-progress requests finish'
            )}
          >
            <SettingsFormGrid>
              <FormField
                control={form.control}
                name='quota_setting.settlement_lead_minutes'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Lead time')}</FormLabel>
                    <FormControl>
                      <Input
                        type='number'
                        min={3}
                        max={60}
                        step={1}
                        {...field}
                      />
                    </FormControl>
                    <FormDescription>
                      {t('Minutes, from 3 to 60')}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name='quota_setting.settlement_prompt'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Message')}</FormLabel>
                    <FormControl>
                      <Textarea rows={3} maxLength={200} {...field} />
                    </FormControl>
                    <FormDescription>
                      {t(
                        'Managed users see this message when starting a new request during settlement'
                      )}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </SettingsFormGrid>
          </SettingsFormSection>

          <Controller
            control={form.control}
            name='quota_setting.temporary_quota_projects'
            render={({ field, fieldState }) => (
              <TemporaryQuotaProjectsEditor
                value={field.value}
                onChange={field.onChange}
                disabled={updateOption.isPending || isSubmitting}
                validationError={fieldState.error?.message}
              />
            )}
          />
        </SettingsForm>
      </Form>
    </SettingsSection>
  )
}

function getCycleQuotaSectionContent(
  _section: 'settings',
  settings: CycleQuotaSettings
) {
  return (
    <CycleQuotaSettingsForm
      defaultValues={{
        quota_setting: {
          settlement_lead_minutes:
            settings['quota_setting.settlement_lead_minutes'],
          settlement_prompt: settings['quota_setting.settlement_prompt'],
          temporary_quota_projects:
            settings['quota_setting.temporary_quota_projects'],
        },
      }}
    />
  )
}

export function CycleQuotaSettingsPage() {
  return (
    <SettingsPage
      routePath='/_authenticated/system-settings/cycle-quota'
      defaultSettings={defaultCycleQuotaSettings}
      defaultSection='settings'
      getSectionContent={getCycleQuotaSectionContent}
      getSectionMeta={() => ({ titleKey: 'Cycle Quota Management' })}
      loadingMessage='Loading cycle quota management settings'
    />
  )
}
