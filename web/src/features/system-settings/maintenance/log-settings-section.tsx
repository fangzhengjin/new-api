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
import { useEffect, useId, useMemo } from 'react'
import { useFieldArray, useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import * as z from 'zod'

import { StaticDataTable } from '@/components/data-table/static/static-data-table'
import {
  Accordion,
  AccordionContent,
  AccordionItem,
  AccordionTrigger,
} from '@/components/ui/accordion'
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
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { Separator } from '@/components/ui/separator'
import { Switch } from '@/components/ui/switch'
import { TableCell, TableRow } from '@/components/ui/table'

import {
  SettingsForm,
  SettingsFormGrid,
  SettingsSwitchContent,
  SettingsSwitchItem,
} from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useUpdateOption } from '../hooks/use-update-option'
import { safeNumberFieldProps } from '../utils/numeric-field'
import { safeJsonParse } from '../utils/json-parser'
import { HistoryLogCleanup } from './history-log-cleanup'
import { ServerLogFiles } from './server-log-files'

const headerNamePattern = /^[!#$%&'+\-.^_`|~0-9A-Za-z]+$/

type RequestHeaderRuleFormValue = {
  name: string
  record: boolean
  forward: boolean
}

type RequestHeaderRuleTableRow = RequestHeaderRuleFormValue & {
  key: string
  index: number
}

type CDNRequestHeaderRuleGroup = {
  vendor: string
  rules: RequestHeaderRuleFormValue[]
}

type CDNRequestHeaderRuleTableRow = RequestHeaderRuleTableRow & {
  vendor: string
  showVendor: boolean
  vendorRowSpan: number
}

function createLogSettingsSchema(
  t: TFunction,
  systemRules: RequestHeaderRuleFormValue[]
) {
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
    log_setting: z.object({
      user_export_enabled: z.boolean(),
      max_range_days: z.number().int().min(1),
    }),
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
          } else if (
            systemRules.some((rule) =>
              requestHeaderRuleMatches(name, rule.name)
            )
          ) {
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

type LogSettingsFormValues = z.infer<ReturnType<typeof createLogSettingsSchema>>

type LogSettingsSectionProps = {
  defaultValues: {
    LogConsumeEnabled: boolean
    'log_setting.user_export_enabled': boolean
    'log_setting.max_range_days': number
  }
  defaultRules: string
  builtInRules: string
  cdnRuleGroups: string
  systemRules: string
  capacityBytes: number
}

function requestHeaderRuleMatches(name: string, rule: string): boolean {
  const normalizedName = name.trim().toLowerCase()
  const normalizedRule = rule.trim().toLowerCase()
  if (!normalizedName || !normalizedRule) return false
  if (normalizedRule.startsWith('*')) {
    const suffix = normalizedRule.slice(1)
    return Boolean(
      suffix &&
      (normalizedName === suffix || normalizedName.endsWith(`-${suffix}`))
    )
  }
  if (normalizedRule.endsWith('*')) {
    return normalizedName.startsWith(normalizedRule.slice(0, -1))
  }
  return normalizedName === normalizedRule
}

function isRequestHeaderRule(
  value: unknown
): value is RequestHeaderRuleFormValue {
  if (typeof value !== 'object' || value === null) return false
  const rule = value as Record<string, unknown>
  return (
    typeof rule.name === 'string' &&
    typeof rule.record === 'boolean' &&
    typeof rule.forward === 'boolean'
  )
}

function parseRequestHeaderRules(value: string): RequestHeaderRuleFormValue[] {
  const rules = safeJsonParse<unknown>(value, { fallback: [], silent: true })
  if (!Array.isArray(rules)) return []
  return rules.filter(isRequestHeaderRule)
}

function parseCDNRequestHeaderRuleGroups(
  value: string
): CDNRequestHeaderRuleGroup[] {
  const groups = safeJsonParse<unknown>(value, { fallback: [], silent: true })
  if (!Array.isArray(groups)) return []

  return groups
    .filter(
      (group): group is { vendor: string; rules: unknown[] } =>
        typeof group === 'object' &&
        group !== null &&
        typeof group.vendor === 'string' &&
        Array.isArray(group.rules)
    )
    .map((group) => ({
      vendor: group.vendor,
      rules: group.rules.filter(isRequestHeaderRule),
    }))
    .filter((group) => group.rules.length > 0)
}

function prepareEditableRequestHeaderRules(
  rules: RequestHeaderRuleFormValue[],
  systemRules: RequestHeaderRuleFormValue[],
  cdnGroups: CDNRequestHeaderRuleGroup[]
): RequestHeaderRuleFormValue[] {
  const prepared = rules.filter(
    (rule) =>
      !systemRules.some((systemRule) =>
        requestHeaderRuleMatches(rule.name, systemRule.name)
      )
  )
  const presentNames = new Set(prepared.map((rule) => rule.name.toLowerCase()))

  for (const group of cdnGroups) {
    for (const rule of group.rules) {
      if (!presentNames.has(rule.name.toLowerCase())) {
        prepared.push(rule)
        presentNames.add(rule.name.toLowerCase())
      }
    }
  }
  return prepared
}

function serializeRequestHeaderRules(
  rules: RequestHeaderRuleFormValue[]
): string {
  return JSON.stringify(
    rules.map((rule) => ({ ...rule, name: rule.name.trim() }))
  )
}

export function LogSettingsSection(props: LogSettingsSectionProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const recordingTitleId = useId()
  const searchTitleId = useId()
  const systemRules = useMemo(
    () => parseRequestHeaderRules(props.systemRules),
    [props.systemRules]
  )
  const cdnRuleGroups = useMemo(
    () => parseCDNRequestHeaderRuleGroups(props.cdnRuleGroups),
    [props.cdnRuleGroups]
  )
  const defaultRules = useMemo(
    () =>
      prepareEditableRequestHeaderRules(
        parseRequestHeaderRules(props.defaultRules),
        systemRules,
        cdnRuleGroups
      ),
    [cdnRuleGroups, props.defaultRules, systemRules]
  )
  const builtInRules = useMemo(
    () =>
      prepareEditableRequestHeaderRules(
        parseRequestHeaderRules(props.builtInRules),
        systemRules,
        cdnRuleGroups
      ),
    [cdnRuleGroups, props.builtInRules, systemRules]
  )
  const logSettingsSchema = useMemo(
    () => createLogSettingsSchema(t, systemRules),
    [systemRules, t]
  )
  const formDefaults = useMemo<LogSettingsFormValues>(
    () => ({
      LogConsumeEnabled: props.defaultValues.LogConsumeEnabled,
      log_setting: {
        user_export_enabled:
          props.defaultValues['log_setting.user_export_enabled'],
        max_range_days: props.defaultValues['log_setting.max_range_days'],
      },
      RequestHeaderRules: defaultRules,
    }),
    [defaultRules, props.defaultValues]
  )
  const form = useForm<LogSettingsFormValues>({
    resolver: zodResolver(logSettingsSchema),
    defaultValues: formDefaults,
  })
  const { fields, append, remove, replace } = useFieldArray({
    control: form.control,
    name: 'RequestHeaderRules',
  })

  useEffect(() => {
    form.reset(formDefaults)
  }, [formDefaults, form])

  const onSubmit = async (values: LogSettingsFormValues) => {
    const changes = [
      ['LogConsumeEnabled', values.LogConsumeEnabled],
      [
        'log_setting.user_export_enabled',
        values.log_setting.user_export_enabled,
      ],
      ['log_setting.max_range_days', values.log_setting.max_range_days],
    ] as const
    for (const [key, value] of changes) {
      if (value === props.defaultValues[key]) continue
      await updateOption.mutateAsync({ key, value })
    }
    const requestHeaderRules = serializeRequestHeaderRules(
      values.RequestHeaderRules
    )
    if (requestHeaderRules !== serializeRequestHeaderRules(defaultRules)) {
      await updateOption.mutateAsync({
        key: 'RequestHeaderRules',
        value: requestHeaderRules,
      })
    }
  }

  const requestHeaderRules = form.watch('RequestHeaderRules')
  const requestHeaderRulesError = form.formState.errors.RequestHeaderRules
  const hasBuiltInHeaderDefaults = builtInRules.length > 0
  const headerRulesUseBuiltInDefaults =
    serializeRequestHeaderRules(requestHeaderRules) ===
    serializeRequestHeaderRules(builtInRules)
  const requestHeaderRuleRows = requestHeaderRules.map((rule, index) => ({
    ...rule,
    key: fields[index]?.id ?? `user-${index}`,
    index,
  }))
  const cdnRuleNames = new Set(
    cdnRuleGroups.flatMap((group) =>
      group.rules.map((rule) => rule.name.toLowerCase())
    )
  )
  const generalRequestHeaderRuleRows: RequestHeaderRuleTableRow[] =
    requestHeaderRuleRows.filter(
      (rule) => !cdnRuleNames.has(rule.name.toLowerCase())
    )
  const requestHeaderRuleRowsByName = new Map(
    requestHeaderRuleRows.map((rule) => [rule.name.toLowerCase(), rule])
  )
  const cdnRequestHeaderRuleRows: CDNRequestHeaderRuleTableRow[] =
    cdnRuleGroups.flatMap((group) =>
      group.rules.flatMap((managedRule, index) => {
        const rule = requestHeaderRuleRowsByName.get(
          managedRule.name.toLowerCase()
        )
        if (!rule) return []
        return [
          {
            ...rule,
            vendor: group.vendor,
            showVendor: index === 0,
            vendorRowSpan: group.rules.length,
          },
        ]
      })
    )
  const nonForwardedHeaderRules = systemRules.filter((rule) => !rule.forward)
  const forwardableHeaderRules = systemRules.filter(
    (rule) => !rule.record && rule.forward
  )

  const renderRequestHeaderPolicyCheckbox = (
    row: RequestHeaderRuleTableRow,
    policy: 'record' | 'forward'
  ) => (
    <div className='flex justify-center'>
      <Checkbox
        checked={row[policy]}
        aria-label={t(
          policy === 'record' ? 'Record {{name}}' : 'Forward {{name}}',
          {
            name: row.name,
          }
        )}
        onCheckedChange={(checked) => {
          form.setValue(
            `RequestHeaderRules.${row.index}.${policy}`,
            checked === true,
            { shouldDirty: true, shouldValidate: true }
          )
        }}
      />
    </div>
  )

  const restoreHeaderDefaults = () => {
    replace(builtInRules)
    void form.trigger('RequestHeaderRules')
  }

  const addRequestHeaderRule = () => {
    append({ name: '', record: false, forward: false }, { shouldFocus: true })
  }

  return (
    <SettingsSection title={t('Log Maintenance')} className='gap-6'>
      <Form {...form}>
        <SettingsForm onSubmit={form.handleSubmit(onSubmit)}>
          <SettingsPageFormActions
            onSave={form.handleSubmit(onSubmit)}
            isSaving={updateOption.isPending}
            saveLabel='Save log settings'
          />
          <div
            role='group'
            aria-labelledby={recordingTitleId}
            className='flex min-w-0 flex-col gap-4'
          >
            <h4 id={recordingTitleId} className='font-medium'>
              {t('Log recording')}
            </h4>
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
          </div>

          <Separator />

          <div
            role='group'
            aria-labelledby={searchTitleId}
            className='flex min-w-0 flex-col gap-4'
          >
            <h4 id={searchTitleId} className='font-medium'>
              {t('Log search and export')}
            </h4>
            <SettingsFormGrid>
              <FormField
                control={form.control}
                name='log_setting.user_export_enabled'
                render={({ field }) => (
                  <SettingsSwitchItem>
                    <SettingsSwitchContent>
                      <FormLabel>{t('Allow users to export logs')}</FormLabel>
                      <FormDescription>
                        {t(
                          'Users can download their own logs from the usage log page. Administrators can always export.'
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
              <FormField
                control={form.control}
                name='log_setting.max_range_days'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Longest log time range (days)')}</FormLabel>
                    <FormControl>
                      <Input
                        type='number'
                        min={1}
                        step={1}
                        {...safeNumberFieldProps(field)}
                      />
                    </FormControl>
                    <FormDescription>
                      {t(
                        'The longest time range a user can view at once in the logs and on the dashboard, and the longest range any export can cover.'
                      )}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </SettingsFormGrid>
          </div>
        </SettingsForm>
      </Form>

      <Separator />

      <HistoryLogCleanup />

      <Separator />

      <ServerLogFiles />

      <Separator />

      <Form {...form}>
        <SettingsForm onSubmit={form.handleSubmit(onSubmit)}>
          <Card data-settings-form-span='full'>
            <CardHeader className='bg-muted/20 border-b'>
              <div className='flex flex-col gap-2 sm:flex-row'>
                <div className='min-w-0 flex-1'>
                  <CardTitle>
                    <h3>{t('Request header processing rules')}</h3>
                  </CardTitle>
                  <CardDescription>
                    {t(
                      'Manage whether request headers are recorded in administrator logs and sent to upstream providers by source'
                    )}
                  </CardDescription>
                </div>
                <div className='shrink-0 sm:justify-self-end'>
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
                </div>
              </div>
            </CardHeader>
            <CardContent className='space-y-6'>
              <section aria-labelledby='general-request-header-rules'>
                <div className='mb-3 flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between'>
                  <div className='space-y-1'>
                    <div className='flex items-center gap-2'>
                      <h4
                        id='general-request-header-rules'
                        className='font-medium'
                      >
                        {t('General request headers')}
                      </h4>
                      <Badge variant='secondary'>
                        {t('{{count}} rules', {
                          count: generalRequestHeaderRuleRows.length,
                        })}
                      </Badge>
                    </div>
                    <p className='text-muted-foreground text-xs'>
                      {t(
                        'Manage request headers used for log analysis and tracing, and add custom rules'
                      )}
                    </p>
                  </div>
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
                <StaticDataTable
                  data={generalRequestHeaderRuleRows}
                  getRowKey={(row) => row.key}
                  emptyContent={t('No rules yet')}
                  columns={[
                    {
                      id: 'name',
                      header: t('Header rule'),
                      className: 'min-w-60',
                      cell: (row) => {
                        const error =
                          form.formState.errors.RequestHeaderRules?.[row.index]
                            ?.name
                        const label = t('Header rule: {{name}}', {
                          name: row.name || row.index + 1,
                        })
                        return (
                          <div className='space-y-1'>
                            <Input
                              {...form.register(
                                `RequestHeaderRules.${row.index}.name`
                              )}
                              aria-label={label}
                              aria-invalid={Boolean(error)}
                              spellCheck={false}
                            />
                            {error?.message ? (
                              <p
                                className='text-destructive text-xs'
                                role='alert'
                              >
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
                      cell: (row) =>
                        renderRequestHeaderPolicyCheckbox(row, 'record'),
                    },
                    {
                      id: 'forward',
                      header: t('Forward'),
                      className: 'w-24 text-center',
                      cellClassName: 'text-center',
                      cell: (row) =>
                        renderRequestHeaderPolicyCheckbox(row, 'forward'),
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
                          disabled={updateOption.isPending}
                          aria-label={t('Delete {{name}} rule', {
                            name: row.name,
                          })}
                          onClick={() => remove(row.index)}
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
              </section>

              <Separator />

              <section aria-labelledby='cdn-request-header-rules'>
                <div className='mb-3 space-y-1'>
                  <div className='flex items-center gap-2'>
                    <h4 id='cdn-request-header-rules' className='font-medium'>
                      {t('CDN request headers')}
                    </h4>
                    <Badge variant='outline'>
                      {t('{{count}} rules', {
                        count: cdnRequestHeaderRuleRows.length,
                      })}
                    </Badge>
                  </div>
                  <p className='text-muted-foreground text-xs'>
                    {t(
                      'Rule names are maintained by vendor, while administrators can change record and forward policies'
                    )}
                  </p>
                </div>
                <StaticDataTable
                  data={cdnRequestHeaderRuleRows}
                  getRowKey={(row) => row.key}
                  emptyContent={t('No rules yet')}
                  columns={[
                    { id: 'vendor', header: t('Vendor'), className: 'w-52' },
                    {
                      id: 'name',
                      header: t('Header rule'),
                      className: 'min-w-60',
                    },
                    {
                      id: 'record',
                      header: t('Record'),
                      className: 'w-24 text-center',
                    },
                    {
                      id: 'forward',
                      header: t('Forward'),
                      className: 'w-24 text-center',
                    },
                  ]}
                  renderRow={(row) => (
                    <TableRow>
                      {row.showVendor ? (
                        <TableCell
                          rowSpan={row.vendorRowSpan}
                          className='text-muted-foreground align-top font-medium'
                        >
                          {row.vendor}
                        </TableCell>
                      ) : null}
                      <TableCell>
                        <code>{row.name}</code>
                      </TableCell>
                      <TableCell className='text-center'>
                        {renderRequestHeaderPolicyCheckbox(row, 'record')}
                      </TableCell>
                      <TableCell className='text-center'>
                        {renderRequestHeaderPolicyCheckbox(row, 'forward')}
                      </TableCell>
                    </TableRow>
                  )}
                />
              </section>

              {requestHeaderRulesError?.message ? (
                <p className='text-destructive text-xs' role='alert'>
                  {requestHeaderRulesError.message}
                </p>
              ) : null}

              {systemRules.length > 0 ? (
                <>
                  <Separator />
                  <section aria-labelledby='system-request-header-rules'>
                    <div className='mb-3 space-y-1'>
                      <div className='flex items-center gap-2'>
                        <h4
                          id='system-request-header-rules'
                          className='font-medium'
                        >
                          {t('System-protected request headers')}
                        </h4>
                        <Badge variant='outline'>
                          {t('{{count}} rules', {
                            count: systemRules.length,
                          })}
                        </Badge>
                      </div>
                      <p className='text-muted-foreground text-xs'>
                        {t(
                          'These rules protect HTTP transport and credentials and cannot be changed'
                        )}
                      </p>
                    </div>
                    <Accordion className='rounded-lg border px-3'>
                      <AccordionItem value='system-protections'>
                        <AccordionTrigger>
                          {t('View system-managed request headers')}
                        </AccordionTrigger>
                        <AccordionContent className='grid gap-4 lg:grid-cols-2'>
                          {nonForwardedHeaderRules.length > 0 ? (
                            <div className='space-y-2 rounded-lg border p-3'>
                              <div>
                                <h5 className='font-medium'>
                                  {t('Not recorded · Not forwarded')}
                                </h5>
                                <p className='text-muted-foreground text-xs'>
                                  {t(
                                    'These headers are neither recorded nor forwarded'
                                  )}
                                </p>
                              </div>
                              <div className='flex flex-wrap gap-1.5'>
                                {nonForwardedHeaderRules.map((rule) => (
                                  <code
                                    key={rule.name}
                                    className='bg-muted rounded px-1.5 py-0.5 text-xs'
                                  >
                                    {rule.name}
                                  </code>
                                ))}
                              </div>
                            </div>
                          ) : null}
                          {forwardableHeaderRules.length > 0 ? (
                            <div className='space-y-2 rounded-lg border p-3'>
                              <div>
                                <h5 className='font-medium'>
                                  {t('Not recorded · Forwardable')}
                                </h5>
                                <p className='text-muted-foreground text-xs'>
                                  {t(
                                    'These headers are never recorded and may be forwarded when required'
                                  )}
                                </p>
                              </div>
                              <div className='flex flex-wrap gap-1.5'>
                                {forwardableHeaderRules.map((rule) => (
                                  <code
                                    key={rule.name}
                                    className='bg-muted rounded px-1.5 py-0.5 text-xs'
                                  >
                                    {rule.name}
                                  </code>
                                ))}
                              </div>
                            </div>
                          ) : null}
                        </AccordionContent>
                      </AccordionItem>
                    </Accordion>
                  </section>
                </>
              ) : null}

              <div className='text-muted-foreground flex flex-col gap-1 border-t pt-4 text-xs sm:flex-row sm:items-center sm:justify-between'>
                <span>
                  {t(
                    'Unmatched request headers are recorded and forwarded by default'
                  )}
                </span>
                <span className='flex flex-col gap-1 sm:items-end'>
                  <span>
                    {t(
                      'Recorded request headers are visible only to administrators'
                    )}
                  </span>
                  <span className='tabular-nums'>
                    {props.capacityBytes > 0
                      ? t(
                          '{{size}} KiB for incoming and outgoing headers separately',
                          {
                            size: Math.round(props.capacityBytes / 1024),
                          }
                        )
                      : t('Not available')}
                  </span>
                </span>
              </div>
            </CardContent>
          </Card>
        </SettingsForm>
      </Form>
    </SettingsSection>
  )
}
