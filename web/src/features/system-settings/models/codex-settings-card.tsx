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
  clientVersionSchema,
  formatModelPatterns,
  modelPatternListSchema,
  serializeModelPatterns,
} from './client-identity-settings'
import {
  codexErrorResponseMappingSchema,
  codexErrorResponseMappingsFieldSchema,
  formatCodexErrorResponseMappings,
  normalizeCodexErrorResponseMappings,
} from './codex-error-response-mapping'
import { CodexErrorResponseMappings } from './codex-error-response-mappings'

const userAgentComponent = z
  .string()
  .trim()
  .regex(/^[0-9A-Za-z][0-9A-Za-z ._/-]{0,63}$/, {
    message: '请输入有效的 User-Agent 组成字段',
  })

const userAgentClient = z
  .string()
  .trim()
  .regex(/^[0-9A-Za-z][0-9A-Za-z._-]{0,63}$/, {
    message: '请输入有效的 User-Agent 组成字段',
  })

const schema = z.object({
  codex: z.object({
    client_version_check_enabled: z.boolean(),
    minimum_client_version: clientVersionSchema,
    desktop_client_version_check_enabled: z.boolean(),
    minimum_desktop_client_version: clientVersionSchema,
    request_header_fallback_enabled: z.boolean(),
    request_header_fallback_client: userAgentClient,
    request_header_fallback_version: clientVersionSchema,
    request_header_fallback_os: userAgentComponent,
    request_header_fallback_os_version: userAgentComponent,
    request_header_fallback_architecture: userAgentComponent,
    request_header_fallback_terminal: userAgentComponent,
    request_header_model_patterns: modelPatternListSchema,
    error_response_mappings: codexErrorResponseMappingsFieldSchema,
  }),
})

const builtInSchema = z.object({
  codex: schema.shape.codex.extend({
    request_header_model_patterns: z
      .array(z.string())
      .transform((patterns) => JSON.stringify(patterns)),
    error_response_mappings: z
      .array(codexErrorResponseMappingSchema)
      .transform((mappings) => JSON.stringify(mappings)),
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
  'codex.error_response_mappings': string
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
    'codex.error_response_mappings': normalizeCodexErrorResponseMappings(
      values.codex.error_response_mappings
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
      error_response_mappings: formatCodexErrorResponseMappings(
        values.codex.error_response_mappings
      ),
    },
  }
}

/** Renders global Codex client compatibility settings. */
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

    for (const key of changedKeys) {
      await updateOption.mutateAsync({ key, value: normalized[key] })
    }
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
            title={t('Codex 客户端版本校验')}
            description={t('分别配置 Codex CLI 和 Codex Desktop 的最低允许版本')}
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
                          {t('Codex CLI 客户端版本校验')}
                        </FormLabel>
                        <FormDescription>
                          {t('通过 User-Agent 识别 Codex CLI，并拒绝低于最低版本的请求')}
                        </FormDescription>
                      </SettingsSwitchContent>
                      <FormControl>
                        <Switch
                          aria-label={t('Codex CLI 客户端版本校验')}
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
                      <FormLabel>{t('Codex CLI 最低版本')}</FormLabel>
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
                          {t('Codex Desktop 客户端版本校验')}
                        </FormLabel>
                        <FormDescription>
                          {t('通过 User-Agent 中的应用版本识别 Codex Desktop，并拒绝低于最低版本的请求')}
                        </FormDescription>
                      </SettingsSwitchContent>
                      <FormControl>
                        <Switch
                          aria-label={t('Codex Desktop 客户端版本校验')}
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
                        {t('Codex Desktop 最低版本')}
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
            title={t('Codex 客户端身份模拟')}
            description={t('对匹配的映射后上游模型，将其他客户端身份替换为下方配置的 Codex 身份')}
            action={
              <FormField
                control={form.control}
                name='codex.request_header_fallback_enabled'
                render={({ field }) => (
                  <FormItem className='space-y-0'>
                    <FormControl>
                      <Switch
                        aria-label={t('Codex 客户端身份模拟')}
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
                {t('Codex 客户端身份模拟')}
              </FieldLegend>
              <SettingsFormGrid>
                <FormField
                  control={form.control}
                  name='codex.request_header_fallback_client'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('客户端标识')}</FormLabel>
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
                      <FormLabel>{t('客户端版本')}</FormLabel>
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
                      <FormLabel>{t('操作系统')}</FormLabel>
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
                      <FormLabel>{t('架构')}</FormLabel>
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
                      <FormLabel>{t('终端')}</FormLabel>
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
                      <FormLabel>{t('目标模型规则')}</FormLabel>
                      <FormControl>
                        <Textarea {...field} rows={3} />
                      </FormControl>
                      <FormDescription>
                        {t('每行一个正则表达式，匹配实际发送给上游的映射后模型名称')}
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              </SettingsFormGrid>
              <div className='bg-muted/40 rounded-lg p-3'>
                <p className='text-muted-foreground text-xs font-medium'>
                  {t('生成的 User-Agent')}
                </p>
                <code className='mt-1 block text-xs break-all'>
                  {generatedUserAgent}
                </code>
              </div>
            </FieldSet>
          </SettingsFormSection>

          <Separator />

          <FormField
            control={form.control}
            name='codex.error_response_mappings'
            render={({ field, fieldState }) => (
              <FormItem data-settings-form-span='full' className='min-w-0'>
                <CodexErrorResponseMappings
                  value={field.value}
                  onChange={field.onChange}
                  onBlur={field.onBlur}
                  invalid={fieldState.invalid}
                />
              </FormItem>
            )}
          />
        </SettingsForm>
      </Form>
    </SettingsSection>
  )
}
