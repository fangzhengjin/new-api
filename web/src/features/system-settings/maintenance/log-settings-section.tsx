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
import { Add01Icon, Delete02Icon, ReloadIcon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import type { TFunction } from 'i18next'
import { useCallback, useEffect, useMemo, useState } from 'react'
import { useFieldArray, useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import * as z from 'zod'

import { StaticDataTable } from '@/components/data-table/static/static-data-table'
import { DateTimePicker } from '@/components/datetime-picker'
import { Alert, AlertDescription } from '@/components/ui/alert'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from '@/components/ui/alert-dialog'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Progress } from '@/components/ui/progress'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Separator } from '@/components/ui/separator'
import { Switch } from '@/components/ui/switch'
import { api } from '@/lib/api'
import dayjs from '@/lib/dayjs'
import { formatTimestampToDate } from '@/lib/format'

import {
  getCurrentLogCleanupTask,
  getSystemTask,
  startLogCleanupTask,
} from '../api'
import {
  SettingsControlGroup,
  SettingsForm,
  SettingsFormSection,
  SettingsSwitchContent,
  SettingsSwitchItem,
} from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useUpdateOption } from '../hooks/use-update-option'
import type { LogCleanupTask } from '../types'
import { safeJsonParse } from '../utils/json-parser'

const headerNamePattern = /^[!#$%&'+\-.^_`|~0-9A-Za-z]+$/

type RequestHeaderRuleFormValue = {
  name: string
  record: boolean
  forward: boolean
}

type LogSettingsFormValues = {
  LogConsumeEnabled: boolean
  RequestHeaderRules: RequestHeaderRuleFormValue[]
}

type RequestHeaderRuleTableRow = RequestHeaderRuleFormValue & {
  key: string
  index: number
  system: boolean
}

function createLogSettingsSchema(
  t: TFunction,
  systemRules: RequestHeaderRuleFormValue[]
) {
  const systemRuleNames = new Set(
    systemRules.map((rule) => rule.name.toLowerCase())
  )
  const ruleSchema = z.object({
    name: z
      .string()
      .trim()
      .min(1, t('Request header rule is required'))
      .refine((value) => {
        const name = value.endsWith('*') ? value.slice(0, -1) : value
        return Boolean(
          name && !name.includes('*') && headerNamePattern.test(name)
        )
      }, t('Use an HTTP header name; only a trailing * wildcard is supported')),
    record: z.boolean(),
    forward: z.boolean(),
  })

  return z.object({
    LogConsumeEnabled: z.boolean(),
    RequestHeaderRules: z
      .array(ruleSchema)
      .max(200, t('Request header rules cannot exceed 200 rules'))
      .superRefine((rules, context) => {
        if (JSON.stringify(rules).length > 8192) {
          context.addIssue({
            code: 'custom',
            message: t('Request header rules cannot exceed 8 KiB'),
          })
        }
        const seen = new Set<string>()
        rules.forEach((rule, index) => {
          const name = rule.name.trim().toLowerCase()
          if (seen.has(name)) {
            context.addIssue({
              code: 'custom',
              path: [index, 'name'],
              message: t('Duplicate request header rule'),
            })
          } else if (systemRuleNames.has(name)) {
            context.addIssue({
              code: 'custom',
              path: [index, 'name'],
              message: t('This request header rule is managed by the system'),
            })
          }
          seen.add(name)
        })
      }),
  })
}

type LogSettingsSectionProps = {
  defaultEnabled: boolean
  defaultRules: string
  builtInRules: string
  systemRules: string
  capacityBytes: number
}

type ServerLogInfo = {
  enabled: boolean
  log_dir: string
  file_count: number
  total_size: number
  oldest_time?: string
  newest_time?: string
}

const HOURS_IN_DAY = 24

function parseRequestHeaderRules(value: string): RequestHeaderRuleFormValue[] {
  const rules = safeJsonParse<unknown>(value, { fallback: [], silent: true })
  if (!Array.isArray(rules)) return []
  return rules.filter(
    (rule): rule is RequestHeaderRuleFormValue =>
      typeof rule === 'object' &&
      rule !== null &&
      typeof rule.name === 'string' &&
      typeof rule.record === 'boolean' &&
      typeof rule.forward === 'boolean'
  )
}

function serializeRequestHeaderRules(
  rules: RequestHeaderRuleFormValue[]
): string {
  return JSON.stringify(
    rules.map((rule) => ({ ...rule, name: rule.name.trim() }))
  )
}

function formatBytes(bytes: number, decimals = 2): string {
  if (!bytes || Number.isNaN(bytes)) return '0 Bytes'
  if (bytes === 0) return '0 Bytes'
  if (bytes < 0) return `-${formatBytes(-bytes, decimals)}`
  const k = 1024
  const sizes = ['Bytes', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(Math.abs(bytes)) / Math.log(k))
  if (i < 0 || i >= sizes.length) return `${bytes} Bytes`
  return `${Number.parseFloat((bytes / Math.pow(k, i)).toFixed(decimals))} ${
    sizes[i]
  }`
}

const getDateHoursAgo = (hours: number) => {
  const date = new Date()
  date.setHours(date.getHours() - hours)
  return date
}

const getDateDaysAgo = (days: number) => getDateHoursAgo(days * HOURS_IN_DAY)

const quickSelectOptions = [
  {
    label: '24 hours ago',
    getValue: () => getDateHoursAgo(24),
  },
  {
    label: '7 days ago',
    getValue: () => getDateDaysAgo(7),
  },
  {
    label: '30 days ago',
    getValue: () => getDateDaysAgo(30),
  },
]

function isActiveLogCleanupTask(task: LogCleanupTask | null) {
  return task?.status === 'pending' || task?.status === 'running'
}

export function LogSettingsSection(props: LogSettingsSectionProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const defaultRules = useMemo(
    () => parseRequestHeaderRules(props.defaultRules),
    [props.defaultRules]
  )
  const builtInRules = useMemo(
    () => parseRequestHeaderRules(props.builtInRules),
    [props.builtInRules]
  )
  const systemRules = useMemo(
    () => parseRequestHeaderRules(props.systemRules),
    [props.systemRules]
  )
  const logSettingsSchema = useMemo(
    () => createLogSettingsSchema(t, systemRules),
    [systemRules, t]
  )
  const form = useForm<LogSettingsFormValues>({
    resolver: zodResolver(logSettingsSchema),
    defaultValues: {
      LogConsumeEnabled: props.defaultEnabled,
      RequestHeaderRules: defaultRules,
    },
  })
  const { fields, append, remove, replace } = useFieldArray({
    control: form.control,
    name: 'RequestHeaderRules',
  })

  const [purgeDate, setPurgeDate] = useState<Date | undefined>(() =>
    getDateDaysAgo(30)
  )
  const [isStartingLogCleanup, setIsStartingLogCleanup] = useState(false)
  const [logCleanupTask, setLogCleanupTask] = useState<LogCleanupTask | null>(
    null
  )
  const [showConfirmDialog, setShowConfirmDialog] = useState(false)
  const [serverLogInfo, setServerLogInfo] = useState<ServerLogInfo | null>(null)
  const [serverLogCleanupMode, setServerLogCleanupMode] = useState('by_count')
  const [serverLogCleanupValue, setServerLogCleanupValue] = useState(10)
  const [serverLogCleanupLoading, setServerLogCleanupLoading] = useState(false)

  const fetchServerLogInfo = useCallback(async () => {
    try {
      const res = await api.get('/api/performance/logs')
      if (res.data.success) setServerLogInfo(res.data.data)
    } catch {
      /* ignore */
    }
  }, [])

  useEffect(() => {
    form.reset({
      LogConsumeEnabled: props.defaultEnabled,
      RequestHeaderRules: defaultRules,
    })
  }, [defaultRules, form, props.defaultEnabled])

  useEffect(() => {
    fetchServerLogInfo()
  }, [fetchServerLogInfo])

  useEffect(() => {
    let cancelled = false

    async function fetchCurrentLogCleanupTask() {
      try {
        const res = await getCurrentLogCleanupTask()
        if (!cancelled && res.success && res.data) {
          setLogCleanupTask(res.data)
        }
      } catch {
        /* ignore */
      }
    }

    fetchCurrentLogCleanupTask()

    return () => {
      cancelled = true
    }
  }, [])

  const purgeTimestamp = useMemo(() => {
    if (!purgeDate) return null
    return Math.floor(purgeDate.getTime() / 1000)
  }, [purgeDate])

  const formattedPurgeDate = useMemo(() => {
    if (!purgeDate) return ''
    return formatTimestampToDate(purgeDate.getTime(), 'milliseconds')
  }, [purgeDate])

  const logCleanupActive = isActiveLogCleanupTask(logCleanupTask)
  const logCleanupState = logCleanupTask?.state
  const logCleanupProgress = Math.min(
    100,
    Math.max(0, logCleanupState?.progress ?? 0)
  )
  const logCleanupProcessed = logCleanupState?.processed ?? 0
  const logCleanupTotal = logCleanupState?.total ?? 0
  const logCleanupTaskId = logCleanupTask?.task_id

  useEffect(() => {
    if (!logCleanupTaskId || !logCleanupActive) return

    let cancelled = false
    const interval = window.setInterval(async () => {
      try {
        const res = await getSystemTask(logCleanupTaskId)
        if (cancelled || !res.success || !res.data) return

        setLogCleanupTask(res.data)
        if (!isActiveLogCleanupTask(res.data)) {
          if (res.data.status === 'succeeded') {
            const count =
              res.data.result?.deleted_count ?? res.data.state?.processed ?? 0
            toast.success(
              count > 0
                ? t('{{count}} log entries removed.', { count })
                : t('No log entries matched the selected time.')
            )
          } else if (res.data.status === 'failed') {
            toast.error(res.data.error || t('Failed to clean logs'))
          }
        }
      } catch {
        /* keep polling */
      }
    }, 1000)

    return () => {
      cancelled = true
      window.clearInterval(interval)
    }
  }, [logCleanupActive, logCleanupTaskId, t])

  const onSubmit = async (values: LogSettingsFormValues) => {
    const updates = []
    if (values.LogConsumeEnabled !== props.defaultEnabled) {
      updates.push({
        key: 'LogConsumeEnabled',
        value: values.LogConsumeEnabled,
      })
    }
    const requestHeaderRules = serializeRequestHeaderRules(
      values.RequestHeaderRules
    )
    if (requestHeaderRules !== serializeRequestHeaderRules(defaultRules)) {
      updates.push({
        key: 'RequestHeaderRules',
        value: requestHeaderRules,
      })
    }
    if (updates.length === 0) return
    await updateOption.mutateAsync(updates)
  }

  const requestHeaderRules = form.watch('RequestHeaderRules')
  const requestHeaderRulesError = form.formState.errors.RequestHeaderRules
  const hasBuiltInHeaderDefaults = builtInRules.length > 0
  const headerRulesUseBuiltInDefaults =
    serializeRequestHeaderRules(requestHeaderRules) ===
    serializeRequestHeaderRules(builtInRules)
  const requestHeaderRuleRows: RequestHeaderRuleTableRow[] = [
    ...requestHeaderRules.map((rule, index) => ({
      ...rule,
      key: fields[index]?.id ?? `user-${index}`,
      index,
      system: false,
    })),
    ...systemRules.map((rule, index) => ({
      ...rule,
      key: `system-${rule.name.toLowerCase()}`,
      index,
      system: true,
    })),
  ]

  const restoreHeaderDefaults = () => {
    replace(builtInRules)
    void form.trigger('RequestHeaderRules')
  }

  const addRequestHeaderRule = () => {
    append({ name: '', record: false, forward: false }, { shouldFocus: true })
  }

  const handleRequestCleanLogs = () => {
    if (!purgeTimestamp) {
      toast.error(t('Select a timestamp before clearing logs.'))
      return
    }

    setShowConfirmDialog(true)
  }

  const handleCleanLogs = async () => {
    if (!purgeTimestamp) {
      toast.error(t('Select a timestamp before clearing logs.'))
      return
    }

    setIsStartingLogCleanup(true)
    try {
      const res = await startLogCleanupTask(purgeTimestamp)
      if (!res.success) {
        throw new Error(res.message || t('Failed to clean logs'))
      }
      if (!res.data) {
        throw new Error(t('Failed to clean logs'))
      }
      setLogCleanupTask(res.data)
      setShowConfirmDialog(false)
      toast.success(t('Log cleanup task started.'))
    } catch (error) {
      const message =
        error instanceof Error ? error.message : t('Failed to clean logs')
      toast.error(message)
    } finally {
      setIsStartingLogCleanup(false)
    }
  }

  const cleanupServerLogFiles = async () => {
    if (
      !serverLogCleanupValue ||
      Number.isNaN(serverLogCleanupValue) ||
      serverLogCleanupValue < 1
    ) {
      toast.error(t('Please enter a valid number'))
      return
    }

    setServerLogCleanupLoading(true)
    try {
      const res = await api.delete(
        `/api/performance/logs?mode=${serverLogCleanupMode}&value=${serverLogCleanupValue}`
      )
      if (res.data.success) {
        const { deleted_count, freed_bytes } = res.data.data
        toast.success(
          t('Cleaned up {{count}} log files, freed {{size}}', {
            count: deleted_count,
            size: formatBytes(freed_bytes),
          })
        )
      } else {
        toast.error(res.data.message || t('Cleanup failed'))
      }
      fetchServerLogInfo()
    } catch {
      toast.error(t('Cleanup failed'))
    } finally {
      setServerLogCleanupLoading(false)
    }
  }

  return (
    <SettingsSection title={t('Log Maintenance')}>
      <Form {...form}>
        <SettingsForm onSubmit={form.handleSubmit(onSubmit)}>
          <SettingsPageFormActions
            onSave={form.handleSubmit(onSubmit)}
            isSaving={updateOption.isPending}
            saveLabel='Save log settings'
          />
          <FormField
            control={form.control}
            name='LogConsumeEnabled'
            render={({ field }) => (
              <SettingsSwitchItem>
                <SettingsSwitchContent>
                  <FormLabel>{t('Record quota usage')}</FormLabel>
                  <FormDescription>
                    {t(
                      'Track per-request consumption to power usage analytics. Keeping this on increases database writes.'
                    )}
                  </FormDescription>
                </SettingsSwitchContent>
                <FormControl>
                  <Switch
                    checked={field.value}
                    onCheckedChange={field.onChange}
                  />
                </FormControl>
                <FormMessage />
              </SettingsSwitchItem>
            )}
          />

          <SettingsControlGroup className='space-y-3'>
            <div>
              <h4 className='text-sm font-medium'>{t('Clean history logs')}</h4>
              <p className='text-muted-foreground text-sm'>
                {t(
                  'Remove all log entries created before the selected timestamp.'
                )}
              </p>
            </div>
            <DateTimePicker value={purgeDate} onChange={setPurgeDate} />
            <div className='flex flex-wrap gap-3'>
              {quickSelectOptions.map((option) => (
                <Button
                  key={option.label}
                  type='button'
                  variant='outline'
                  onClick={() => setPurgeDate(option.getValue())}
                >
                  {t(option.label)}
                </Button>
              ))}
              <Button
                type='button'
                variant='destructive'
                onClick={handleRequestCleanLogs}
                disabled={isStartingLogCleanup || logCleanupActive}
              >
                {isStartingLogCleanup || logCleanupActive
                  ? t('Cleaning...')
                  : t('Clean logs')}
              </Button>
            </div>
            {logCleanupTask && (
              <div className='rounded-md border p-3'>
                <div className='mb-2 flex items-center justify-between gap-3 text-sm'>
                  <span className='font-medium'>
                    {t('Log cleanup progress')}
                  </span>
                  <span className='text-muted-foreground tabular-nums'>
                    {logCleanupProgress}%
                  </span>
                </div>
                <Progress value={logCleanupProgress} />
                <div className='text-muted-foreground mt-2 text-xs'>
                  {t('{{processed}} of {{total}} log entries processed.', {
                    processed: logCleanupProcessed,
                    total: logCleanupTotal,
                  })}
                </div>
                {logCleanupTask.status === 'failed' && logCleanupTask.error && (
                  <div className='text-destructive mt-2 text-xs'>
                    {logCleanupTask.error}
                  </div>
                )}
              </div>
            )}
          </SettingsControlGroup>
        </SettingsForm>
      </Form>

      <Separator />

      <div className='space-y-4'>
        <div>
          <h4 className='font-medium'>{t('Server Log Management')}</h4>
          <p className='text-muted-foreground mt-1 text-xs'>
            {t(
              'Manage server log files. Log files accumulate over time; regular cleanup is recommended to free disk space.'
            )}
          </p>
        </div>

        {serverLogInfo !== null &&
          (serverLogInfo.enabled ? (
            <div className='space-y-4'>
              <div className='rounded-lg border p-4'>
                <div className='grid grid-cols-2 gap-2 text-sm md:grid-cols-4'>
                  <div>
                    <span className='text-muted-foreground'>
                      {t('Log Directory')}:
                    </span>{' '}
                    <span className='font-mono text-xs'>
                      {serverLogInfo.log_dir}
                    </span>
                  </div>
                  <div>
                    <span className='text-muted-foreground'>
                      {t('Log File Count')}:
                    </span>{' '}
                    {serverLogInfo.file_count}
                  </div>
                  <div>
                    <span className='text-muted-foreground'>
                      {t('Total Log Size')}:
                    </span>{' '}
                    {formatBytes(serverLogInfo.total_size)}
                  </div>
                  {serverLogInfo.oldest_time && serverLogInfo.newest_time && (
                    <div>
                      <span className='text-muted-foreground'>
                        {t('Date Range')}:
                      </span>{' '}
                      {dayjs(serverLogInfo.oldest_time).format('YYYY-MM-DD')} ~{' '}
                      {dayjs(serverLogInfo.newest_time).format('YYYY-MM-DD')}
                    </div>
                  )}
                </div>
              </div>

              <div className='flex flex-wrap items-end gap-3'>
                <div className='grid gap-1.5'>
                  <Label className='text-xs'>{t('Cleanup Mode')}</Label>
                  <Select
                    items={[
                      { value: 'by_count', label: t('Retain last N files') },
                      { value: 'by_days', label: t('Retain last N days') },
                    ]}
                    value={serverLogCleanupMode}
                    onValueChange={(value) =>
                      value !== null && setServerLogCleanupMode(value)
                    }
                  >
                    <SelectTrigger className='w-[160px]'>
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent alignItemWithTrigger={false}>
                      <SelectGroup>
                        <SelectItem value='by_count'>
                          {t('Retain last N files')}
                        </SelectItem>
                        <SelectItem value='by_days'>
                          {t('Retain last N days')}
                        </SelectItem>
                      </SelectGroup>
                    </SelectContent>
                  </Select>
                </div>
                <div className='grid gap-1.5'>
                  <Label className='text-xs'>
                    {serverLogCleanupMode === 'by_count'
                      ? t('Files to Retain')
                      : t('Days to Retain')}
                  </Label>
                  <Input
                    type='number'
                    min={1}
                    max={serverLogCleanupMode === 'by_count' ? 1000 : 3650}
                    value={serverLogCleanupValue}
                    onChange={(event) =>
                      setServerLogCleanupValue(Number(event.target.value))
                    }
                    className='w-[120px]'
                  />
                </div>
                <AlertDialog>
                  <AlertDialogTrigger
                    render={
                      <Button
                        type='button'
                        variant='destructive'
                        size='sm'
                        disabled={serverLogCleanupLoading}
                      />
                    }
                  >
                    {serverLogCleanupLoading
                      ? t('Cleaning...')
                      : t('Clean Up Log Files')}
                  </AlertDialogTrigger>
                  <AlertDialogContent>
                    <AlertDialogHeader>
                      <AlertDialogTitle>
                        {t('Confirm log file cleanup?')}
                      </AlertDialogTitle>
                      <AlertDialogDescription>
                        {serverLogCleanupMode === 'by_count'
                          ? t(
                              'Only the last {{value}} log files will be retained; the rest will be deleted.',
                              {
                                value: serverLogCleanupValue,
                              }
                            )
                          : t(
                              'Log files older than {{value}} days will be deleted.',
                              {
                                value: serverLogCleanupValue,
                              }
                            )}
                      </AlertDialogDescription>
                    </AlertDialogHeader>
                    <AlertDialogFooter>
                      <AlertDialogCancel>{t('Cancel')}</AlertDialogCancel>
                      <AlertDialogAction
                        variant='destructive'
                        onClick={cleanupServerLogFiles}
                      >
                        {t('Confirm Cleanup')}
                      </AlertDialogAction>
                    </AlertDialogFooter>
                  </AlertDialogContent>
                </AlertDialog>
              </div>
            </div>
          ) : (
            <Alert>
              <AlertDescription>
                {t(
                  'Server logging is not enabled (log directory not configured)'
                )}
              </AlertDescription>
            </Alert>
          ))}
      </div>

      <Separator />

      <Form {...form}>
        <SettingsForm onSubmit={form.handleSubmit(onSubmit)}>
          <SettingsFormSection
            title={t('Request header audit')}
            description={t(
              'Exact rules take precedence; otherwise the wildcard with the longest fixed prefix is used. System rules always apply.'
            )}
            action={
              <div className='flex flex-col gap-2 sm:flex-row'>
                <Button
                  type='button'
                  size='sm'
                  variant='outline'
                  onClick={restoreHeaderDefaults}
                  disabled={
                    updateOption.isPending ||
                    !hasBuiltInHeaderDefaults ||
                    headerRulesUseBuiltInDefaults
                  }
                >
                  <HugeiconsIcon
                    icon={ReloadIcon}
                    strokeWidth={2}
                    data-icon='inline-start'
                    aria-hidden='true'
                  />
                  {t('Restore defaults')}
                </Button>
                <Button
                  type='button'
                  size='sm'
                  onClick={addRequestHeaderRule}
                  disabled={
                    updateOption.isPending || requestHeaderRules.length >= 200
                  }
                >
                  <HugeiconsIcon
                    icon={Add01Icon}
                    strokeWidth={2}
                    data-icon='inline-start'
                    aria-hidden='true'
                  />
                  {t('Add rule')}
                </Button>
              </div>
            }
          >
            <StaticDataTable
              data={requestHeaderRuleRows}
              getRowKey={(row) => row.key}
              getRowClassName={(row) =>
                row.system ? 'bg-muted/30' : undefined
              }
              emptyContent={t('No rules yet')}
              columns={[
                {
                  id: 'name',
                  header: t('Header rule'),
                  className: 'min-w-60',
                  cell: (row) => {
                    const error = row.system
                      ? undefined
                      : form.formState.errors.RequestHeaderRules?.[row.index]
                          ?.name
                    const label = t('Header rule: {{name}}', {
                      name: row.name || row.index + 1,
                    })
                    return (
                      <div className='space-y-1'>
                        {row.system ? (
                          <Input value={row.name} aria-label={label} disabled />
                        ) : (
                          <Input
                            {...form.register(
                              `RequestHeaderRules.${row.index}.name`
                            )}
                            aria-label={label}
                            aria-invalid={Boolean(error)}
                            spellCheck={false}
                          />
                        )}
                        {error?.message ? (
                          <p className='text-destructive text-xs' role='alert'>
                            {error.message}
                          </p>
                        ) : null}
                      </div>
                    )
                  },
                },
                {
                  id: 'record',
                  header: t('Record'),
                  className: 'w-24 text-center',
                  cellClassName: 'text-center',
                  cell: (row) => (
                    <div className='flex justify-center'>
                      <Checkbox
                        checked={row.record}
                        disabled={row.system}
                        aria-label={t('Record {{name}}', { name: row.name })}
                        onCheckedChange={(checked) => {
                          if (row.system) return
                          form.setValue(
                            `RequestHeaderRules.${row.index}.record`,
                            checked === true,
                            { shouldDirty: true, shouldValidate: true }
                          )
                        }}
                      />
                    </div>
                  ),
                },
                {
                  id: 'forward',
                  header: t('Forward'),
                  className: 'w-24 text-center',
                  cellClassName: 'text-center',
                  cell: (row) => (
                    <div className='flex justify-center'>
                      <Checkbox
                        checked={row.forward}
                        disabled={row.system}
                        aria-label={t('Forward {{name}}', { name: row.name })}
                        onCheckedChange={(checked) => {
                          if (row.system) return
                          form.setValue(
                            `RequestHeaderRules.${row.index}.forward`,
                            checked === true,
                            { shouldDirty: true, shouldValidate: true }
                          )
                        }}
                      />
                    </div>
                  ),
                },
                {
                  id: 'actions',
                  header: t('Actions'),
                  className: 'w-20 text-right',
                  cellClassName: 'text-right',
                  cell: (row) => (
                    <Button
                      type='button'
                      variant='ghost'
                      size='icon'
                      disabled={row.system || updateOption.isPending}
                      aria-label={t('Delete {{name}} rule', {
                        name: row.name,
                      })}
                      onClick={() => {
                        if (!row.system) remove(row.index)
                      }}
                    >
                      <HugeiconsIcon
                        icon={Delete02Icon}
                        strokeWidth={2}
                        aria-hidden='true'
                      />
                    </Button>
                  ),
                },
              ]}
            />

            {requestHeaderRulesError?.message ? (
              <p className='text-destructive text-xs' role='alert'>
                {requestHeaderRulesError.message}
              </p>
            ) : null}

            <dl className='grid gap-1 text-xs sm:grid-cols-[auto_1fr] sm:gap-x-4'>
              <dt className='text-muted-foreground'>{t('Audit capacity')}</dt>
              <dd>
                {props.capacityBytes > 0
                  ? t(
                      '{{size}} KiB for incoming and outgoing headers separately',
                      {
                        size: Math.round(props.capacityBytes / 1024),
                      }
                    )
                  : t('Not available')}
              </dd>
            </dl>
            <p className='text-muted-foreground text-xs'>
              {t(
                'Request headers are available only to administrators in log details.'
              )}
            </p>
          </SettingsFormSection>
        </SettingsForm>
      </Form>

      <AlertDialog open={showConfirmDialog} onOpenChange={setShowConfirmDialog}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t('Confirm log cleanup')}</AlertDialogTitle>
            <AlertDialogDescription>
              {formattedPurgeDate
                ? t(
                    'This will permanently remove all log entries created before {{date}}.',
                    { date: formattedPurgeDate }
                  )
                : t(
                    'This will permanently remove log entries before the selected timestamp.'
                  )}{' '}
              {t('This action cannot be undone.')}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={isStartingLogCleanup}>
              {t('Cancel')}
            </AlertDialogCancel>
            <AlertDialogAction
              variant='destructive'
              onClick={handleCleanLogs}
              disabled={isStartingLogCleanup}
            >
              {isStartingLogCleanup ? t('Cleaning...') : t('Delete logs')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </SettingsSection>
  )
}
