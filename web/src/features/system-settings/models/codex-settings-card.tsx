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
import { useEffect, useMemo, useRef } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import * as z from 'zod'

import { FieldLegend, FieldSet } from '@/components/ui/field'
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
import { Textarea } from '@/components/ui/textarea'

import {
  SettingsForm,
  SettingsFormGrid,
  SettingsFormSection,
  SettingsSwitchContent,
  SettingsSwitchItem,
} from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useUpdateOption } from '../hooks/use-update-option'
import {
  formatModelPatterns,
  modelPatternListSchema,
  serializeModelPatterns,
} from './client-identity-settings'

const clientVersion = z
  .string()
  .trim()
  .regex(/^\d+\.\d+\.\d+(?:-[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$/, {
    message: 'Enter a valid semantic version',
  })

const userAgentComponent = z
  .string()
  .trim()
  .regex(/^[0-9A-Za-z][0-9A-Za-z ._/-]{0,63}$/, {
    message: 'Enter a valid User-Agent component',
  })

const userAgentClient = z
  .string()
  .trim()
  .regex(/^[0-9A-Za-z][0-9A-Za-z._-]{0,63}$/, {
    message: 'Enter a valid User-Agent component',
  })

const schema = z.object({
  codex: z.object({
    client_version_check_enabled: z.boolean(),
    minimum_client_version: clientVersion,
    desktop_client_version_check_enabled: z.boolean(),
    minimum_desktop_client_version: clientVersion,
    request_header_fallback_enabled: z.boolean(),
    request_header_fallback_client: userAgentClient,
    request_header_fallback_version: clientVersion,
    request_header_fallback_os: userAgentComponent,
    request_header_fallback_os_version: userAgentComponent,
    request_header_fallback_architecture: userAgentComponent,
    request_header_fallback_terminal: userAgentComponent,
    request_header_model_patterns: modelPatternListSchema,
  }),
})

const builtInSchema = z.object({
  codex: schema.shape.codex.extend({
    request_header_model_patterns: z
      .array(z.string())
      .transform((patterns) => JSON.stringify(patterns)),
  }),
})

type CodexSettingsFormValues = z.infer<typeof schema>

type FlatCodexSettings = {
  'codex.client_version_check_enabled': boolean
  'codex.minimum_client_version': string
  'codex.desktop_client_version_check_enabled': boolean
  'codex.minimum_desktop_client_version': string
  'codex.request_header_fallback_enabled': boolean
  'codex.request_header_fallback_client': string
  'codex.request_header_fallback_version': string
  'codex.request_header_fallback_os': string
  'codex.request_header_fallback_os_version': string
  'codex.request_header_fallback_architecture': string
  'codex.request_header_fallback_terminal': string
  'codex.request_header_model_patterns': string
}

type CodexSettingsCardProps = {
  defaultValues: CodexSettingsFormValues
  builtInDefaults: string
}

function flattenValues(values: CodexSettingsFormValues): FlatCodexSettings {
  return {
    'codex.client_version_check_enabled':
      values.codex.client_version_check_enabled,
    'codex.minimum_client_version': values.codex.minimum_client_version.trim(),
    'codex.desktop_client_version_check_enabled':
      values.codex.desktop_client_version_check_enabled,
    'codex.minimum_desktop_client_version':
      values.codex.minimum_desktop_client_version.trim(),
    'codex.request_header_fallback_enabled':
      values.codex.request_header_fallback_enabled,
    'codex.request_header_fallback_client':
      values.codex.request_header_fallback_client.trim(),
    'codex.request_header_fallback_version':
      values.codex.request_header_fallback_version.trim(),
    'codex.request_header_fallback_os':
      values.codex.request_header_fallback_os.trim(),
    'codex.request_header_fallback_os_version':
      values.codex.request_header_fallback_os_version.trim(),
    'codex.request_header_fallback_architecture':
      values.codex.request_header_fallback_architecture.trim(),
    'codex.request_header_fallback_terminal':
      values.codex.request_header_fallback_terminal.trim(),
    'codex.request_header_model_patterns': serializeModelPatterns(
      values.codex.request_header_model_patterns
    ),
  }
}

function buildFormDefaults(
  values: CodexSettingsFormValues
): CodexSettingsFormValues {
  return {
    codex: {
      ...values.codex,
      request_header_model_patterns: formatModelPatterns(
        values.codex.request_header_model_patterns
      ),
    },
  }
}

/** Renders Codex client version enforcement and upstream identity completion. */
export function CodexSettingsCard(props: CodexSettingsCardProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const baselineRef = useRef(
    flattenValues(buildFormDefaults(props.defaultValues))
  )
  const form = useForm<CodexSettingsFormValues>({
    resolver: zodResolver(schema),
    defaultValues: buildFormDefaults(props.defaultValues),
  })
  const builtInDefaults = useMemo(() => {
    try {
      const parsed = builtInSchema.safeParse({
        codex: JSON.parse(props.builtInDefaults),
      })
      return parsed.success ? buildFormDefaults(parsed.data) : null
    } catch {
      return null
    }
  }, [props.builtInDefaults])

  useEffect(() => {
    const defaults = buildFormDefaults(props.defaultValues)
    baselineRef.current = flattenValues(defaults)
    form.reset(defaults)
  }, [props.defaultValues, form])

  const onSubmit = async (values: CodexSettingsFormValues) => {
    const normalized = flattenValues(values)
    const changedKeys = (
      Object.keys(normalized) as Array<keyof FlatCodexSettings>
    ).filter((key) => normalized[key] !== baselineRef.current[key])
    if (changedKeys.length === 0) {
      toast.info(t('No changes to save'))
      return
    }

    const updates = changedKeys.map((key) => ({
      key,
      value: normalized[key],
    }))
    await updateOption.mutateAsync(updates.length === 1 ? updates[0] : updates)
    baselineRef.current = normalized
    form.reset(values)
  }

  const currentValues = form.watch()
  const currentCodex = currentValues.codex
  const versionCheckEnabled = currentCodex.client_version_check_enabled
  const desktopVersionCheckEnabled =
    currentCodex.desktop_client_version_check_enabled
  const headerFallbackEnabled = currentCodex.request_header_fallback_enabled
  const fallbackClient = currentCodex.request_header_fallback_client
  const fallbackVersion = currentCodex.request_header_fallback_version
  const fallbackOS = currentCodex.request_header_fallback_os
  const fallbackOSVersion = currentCodex.request_header_fallback_os_version
  const fallbackArchitecture = currentCodex.request_header_fallback_architecture
  const fallbackTerminal = currentCodex.request_header_fallback_terminal
  const builtInFlatValues = builtInDefaults && flattenValues(builtInDefaults)
  const usesBuiltInDefaults =
    builtInFlatValues !== null &&
    Object.entries(flattenValues(currentValues)).every(
      ([key, value]) =>
        builtInFlatValues[key as keyof FlatCodexSettings] === value
    )
  const generatedUserAgent = `${fallbackClient.trim()}/${fallbackVersion.trim()} (${fallbackOS.trim()} ${fallbackOSVersion.trim()}; ${fallbackArchitecture.trim()}) ${fallbackTerminal.trim()} (${fallbackClient.trim()}; ${fallbackVersion.trim()})`

  return (
    <SettingsSection title={t('Codex')}>
      <Form {...form}>
        <SettingsForm onSubmit={form.handleSubmit(onSubmit)}>
          <SettingsPageFormActions
            onSave={form.handleSubmit(onSubmit)}
            onReset={() => builtInDefaults && form.reset(builtInDefaults)}
            isSaving={updateOption.isPending}
            isResetDisabled={!builtInDefaults || usesBuiltInDefaults}
            resetLabel='Restore defaults'
          />

          <SettingsFormSection
            title={t('Codex Client Version Validation')}
            description={t(
              'Configure minimum versions for Codex CLI and Codex Desktop independently.'
            )}
          >
            <div className='space-y-8'>
              <div className='space-y-4'>
                <FormField
                  control={form.control}
                  name='codex.client_version_check_enabled'
                  render={({ field }) => (
                    <SettingsSwitchItem className='items-start py-0'>
                      <SettingsSwitchContent>
                        <FormLabel>
                          {t('Codex CLI Client Version Validation')}
                        </FormLabel>
                        <FormDescription>
                          {t(
                            'Identify Codex CLI by its User-Agent and reject requests below the configured minimum version.'
                          )}
                        </FormDescription>
                      </SettingsSwitchContent>
                      <FormControl>
                        <Switch
                          aria-label={t('Codex CLI Client Version Validation')}
                          checked={field.value}
                          onCheckedChange={field.onChange}
                        />
                      </FormControl>
                    </SettingsSwitchItem>
                  )}
                />
                <FormField
                  control={form.control}
                  name='codex.minimum_client_version'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('Minimum Codex CLI Version')}</FormLabel>
                      <FormControl>
                        <Input {...field} disabled={!versionCheckEnabled} />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              </div>

              <div className='space-y-4'>
                <FormField
                  control={form.control}
                  name='codex.desktop_client_version_check_enabled'
                  render={({ field }) => (
                    <SettingsSwitchItem className='items-start py-0'>
                      <SettingsSwitchContent>
                        <FormLabel>
                          {t('Codex Desktop Client Version Validation')}
                        </FormLabel>
                        <FormDescription>
                          {t(
                            'Identify Codex Desktop by its User-Agent application version and reject requests below the configured minimum version.'
                          )}
                        </FormDescription>
                      </SettingsSwitchContent>
                      <FormControl>
                        <Switch
                          aria-label={t(
                            'Codex Desktop Client Version Validation'
                          )}
                          checked={field.value}
                          onCheckedChange={field.onChange}
                        />
                      </FormControl>
                    </SettingsSwitchItem>
                  )}
                />
                <FormField
                  control={form.control}
                  name='codex.minimum_desktop_client_version'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>
                        {t('Minimum Codex Desktop Version')}
                      </FormLabel>
                      <FormControl>
                        <Input
                          {...field}
                          disabled={!desktopVersionCheckEnabled}
                        />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              </div>
            </div>
          </SettingsFormSection>

          <Separator />

          <SettingsFormSection
            title={t('Codex Client Identity Simulation')}
            description={t(
              'For matching mapped upstream models, replace a different client identity with the Codex identity configured below.'
            )}
            action={
              <FormField
                control={form.control}
                name='codex.request_header_fallback_enabled'
                render={({ field }) => (
                  <FormItem className='space-y-0'>
                    <FormControl>
                      <Switch
                        aria-label={t('Codex Client Identity Simulation')}
                        checked={field.value}
                        onCheckedChange={field.onChange}
                      />
                    </FormControl>
                  </FormItem>
                )}
              />
            }
          >
            <FieldSet disabled={!headerFallbackEnabled}>
              <FieldLegend className='sr-only'>
                {t('Codex Client Identity Simulation')}
              </FieldLegend>
              <SettingsFormGrid>
                <FormField
                  control={form.control}
                  name='codex.request_header_fallback_client'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('Client Identifier')}</FormLabel>
                      <FormControl>
                        <Input {...field} />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />
                <FormField
                  control={form.control}
                  name='codex.request_header_fallback_version'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('Client Version')}</FormLabel>
                      <FormControl>
                        <Input {...field} />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />
                <FormField
                  control={form.control}
                  name='codex.request_header_fallback_os'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('Operating System')}</FormLabel>
                      <FormControl>
                        <Input {...field} />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />
                <FormField
                  control={form.control}
                  name='codex.request_header_fallback_os_version'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('System Version')}</FormLabel>
                      <FormControl>
                        <Input {...field} />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />
                <FormField
                  control={form.control}
                  name='codex.request_header_fallback_architecture'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('Architecture')}</FormLabel>
                      <FormControl>
                        <Input {...field} />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />
                <FormField
                  control={form.control}
                  name='codex.request_header_fallback_terminal'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('Terminal')}</FormLabel>
                      <FormControl>
                        <Input {...field} />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />
                <FormField
                  control={form.control}
                  name='codex.request_header_model_patterns'
                  render={({ field }) => (
                    <FormItem className='sm:col-span-2'>
                      <FormLabel>{t('Target Model Patterns')}</FormLabel>
                      <FormControl>
                        <Textarea {...field} rows={3} />
                      </FormControl>
                      <FormDescription>
                        {t(
                          'One regular expression per line. Match the mapped model name that is actually sent upstream.'
                        )}
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              </SettingsFormGrid>
              <div className='bg-muted/40 rounded-lg p-3'>
                <p className='text-muted-foreground text-xs font-medium'>
                  {t('Generated User-Agent')}
                </p>
                <code className='mt-1 block text-xs break-all'>
                  {generatedUserAgent}
                </code>
              </div>
            </FieldSet>
          </SettingsFormSection>
        </SettingsForm>
      </Form>
    </SettingsSection>
  )
}
