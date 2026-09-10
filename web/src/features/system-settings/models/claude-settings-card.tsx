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

import { JsonCodeEditor } from '@/components/json-code-editor'
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
  formatJsonForTextarea,
  normalizeJsonString,
  validateJsonString,
} from './utils'

const schema = z.object({
  claude: z.object({
    client_version_check_enabled: z.boolean(),
    minimum_client_version: clientVersionSchema,
    request_header_fallback_enabled: z.boolean(),
    request_header_fallback_version: clientVersionSchema,
    request_header_model_patterns: modelPatternListSchema,
    model_headers_settings: z.string().superRefine((value, ctx) => {
      const result = validateJsonString(value)
      if (!result.valid) {
        ctx.addIssue({
          code: z.ZodIssueCode.custom,
          message: result.message || 'Invalid JSON',
        })
      }
    }),
    default_max_tokens: z.string().superRefine((value, ctx) => {
      const result = validateJsonString(value)
      if (!result.valid) {
        ctx.addIssue({
          code: z.ZodIssueCode.custom,
          message: result.message || 'Invalid JSON',
        })
      }
    }),
    thinking_adapter_enabled: z.boolean(),
    thinking_adapter_budget_tokens_percentage: z.coerce
      .number()
      .min(0.1, { message: 'Must be at least 0.1' })
      .max(1, { message: 'Must be 1 or less' }),
  }),
})

const builtInSchema = z.object({
  claude: schema.shape.claude.extend({
    request_header_model_patterns: z
      .array(z.string())
      .transform((patterns) => JSON.stringify(patterns)),
    model_headers_settings: z
      .record(z.string(), z.record(z.string(), z.array(z.string())))
      .transform((settings) => JSON.stringify(settings)),
    default_max_tokens: z
      .record(z.string(), z.number().int())
      .transform((settings) => JSON.stringify(settings)),
  }),
})

type ClaudeSettingsFormValues = z.output<typeof schema>
type ClaudeSettingsFormInput = z.input<typeof schema>

type FlatClaudeSettings = {
  'claude.client_version_check_enabled': boolean
  'claude.minimum_client_version': string
  'claude.request_header_fallback_enabled': boolean
  'claude.request_header_fallback_version': string
  'claude.request_header_model_patterns': string
  'claude.model_headers_settings': string
  'claude.default_max_tokens': string
  'claude.thinking_adapter_enabled': boolean
  'claude.thinking_adapter_budget_tokens_percentage': number
}

type ClaudeSettingsCardProps = {
  defaultValues: ClaudeSettingsFormInput
  builtInDefaults: string
}

function buildFormDefaults(
  values: ClaudeSettingsFormInput
): ClaudeSettingsFormInput {
  return {
    claude: {
      client_version_check_enabled: values.claude.client_version_check_enabled,
      minimum_client_version: values.claude.minimum_client_version,
      request_header_fallback_enabled:
        values.claude.request_header_fallback_enabled,
      request_header_fallback_version:
        values.claude.request_header_fallback_version,
      request_header_model_patterns: formatModelPatterns(
        values.claude.request_header_model_patterns
      ),
      model_headers_settings: formatJsonForTextarea(
        values.claude.model_headers_settings
      ),
      default_max_tokens: formatJsonForTextarea(
        values.claude.default_max_tokens
      ),
      thinking_adapter_enabled: values.claude.thinking_adapter_enabled,
      thinking_adapter_budget_tokens_percentage:
        values.claude.thinking_adapter_budget_tokens_percentage,
    },
  }
}

function flattenValues(values: ClaudeSettingsFormInput): FlatClaudeSettings {
  return {
    'claude.client_version_check_enabled':
      values.claude.client_version_check_enabled,
    'claude.minimum_client_version':
      values.claude.minimum_client_version.trim(),
    'claude.request_header_fallback_enabled':
      values.claude.request_header_fallback_enabled,
    'claude.request_header_fallback_version':
      values.claude.request_header_fallback_version.trim(),
    'claude.request_header_model_patterns': serializeModelPatterns(
      values.claude.request_header_model_patterns
    ),
    'claude.model_headers_settings': normalizeJsonString(
      values.claude.model_headers_settings
    ),
    'claude.default_max_tokens': normalizeJsonString(
      values.claude.default_max_tokens
    ),
    'claude.thinking_adapter_enabled': values.claude.thinking_adapter_enabled,
    'claude.thinking_adapter_budget_tokens_percentage': Number(
      values.claude.thinking_adapter_budget_tokens_percentage
    ),
  }
}

export function ClaudeSettingsCard(props: ClaudeSettingsCardProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const normalizedDefaultsRef = useRef(
    flattenValues(buildFormDefaults(props.defaultValues))
  )

  const form = useForm<
    ClaudeSettingsFormInput,
    unknown,
    ClaudeSettingsFormValues
  >({
    resolver: zodResolver(schema),
    defaultValues: buildFormDefaults(props.defaultValues),
  })
  const builtInDefaults = useMemo(() => {
    try {
      const parsed = builtInSchema.safeParse({
        claude: JSON.parse(props.builtInDefaults),
      })
      return parsed.success ? buildFormDefaults(parsed.data) : null
    } catch {
      return null
    }
  }, [props.builtInDefaults])

  useEffect(() => {
    const defaults = buildFormDefaults(props.defaultValues)
    normalizedDefaultsRef.current = flattenValues(defaults)
    form.reset(defaults)
  }, [props.defaultValues, form])

  const onSubmit = async (values: ClaudeSettingsFormValues) => {
    const normalized = flattenValues(values)

    const changedKeys = (
      Object.keys(normalized) as Array<keyof FlatClaudeSettings>
    ).filter((key) => normalized[key] !== normalizedDefaultsRef.current[key])

    if (changedKeys.length === 0) {
      toast.info(t('No changes to save'))
      return
    }

    const updates = changedKeys.map((key) => ({ key, value: normalized[key] }))
    await updateOption.mutateAsync(updates.length === 1 ? updates[0] : updates)
    normalizedDefaultsRef.current = normalized
    form.reset(values)
  }

  const currentValues = form.watch()
  const currentClaude = currentValues.claude
  const versionCheckEnabled = currentClaude.client_version_check_enabled
  const headerFallbackEnabled = currentClaude.request_header_fallback_enabled
  const identityVersion = currentClaude.request_header_fallback_version
  const builtInFlatValues = builtInDefaults && flattenValues(builtInDefaults)
  const usesBuiltInDefaults =
    builtInFlatValues !== null &&
    Object.entries(flattenValues(currentValues)).every(
      ([key, value]) =>
        builtInFlatValues[key as keyof FlatClaudeSettings] === value
    )

  return (
    <SettingsSection title={t('Claude')}>
      <Form {...form}>
        {/* eslint-disable-next-line react-hooks/refs */}
        <SettingsForm onSubmit={form.handleSubmit(onSubmit)}>
          <SettingsPageFormActions
            onSave={form.handleSubmit(onSubmit)}
            onReset={() => builtInDefaults && form.reset(builtInDefaults)}
            isSaving={updateOption.isPending}
            isResetDisabled={!builtInDefaults || usesBuiltInDefaults}
            resetLabel='Restore defaults'
          />

          <SettingsFormSection
            title={t('Claude Code Client Version Validation')}
            description={t(
              'Validate the minimum version for Claude Code and Claude Desktop Code mode.'
            )}
            action={
              <FormField
                control={form.control}
                name='claude.client_version_check_enabled'
                render={({ field }) => (
                  <FormItem className='space-y-0'>
                    <FormControl>
                      <Switch
                        aria-label={t('Claude Code Client Version Validation')}
                        checked={field.value}
                        onCheckedChange={field.onChange}
                      />
                    </FormControl>
                  </FormItem>
                )}
              />
            }
          >
            <FormField
              control={form.control}
              name='claude.minimum_client_version'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Minimum Allowed Version')}</FormLabel>
                  <FormControl>
                    <Input {...field} disabled={!versionCheckEnabled} />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
          </SettingsFormSection>

          <Separator />

          <SettingsFormSection
            title={t('Claude Code Client Identity Simulation')}
            description={t(
              'For matching mapped upstream models, replace a different client identity with Claude Code identity.'
            )}
            action={
              <FormField
                control={form.control}
                name='claude.request_header_fallback_enabled'
                render={({ field }) => (
                  <FormItem className='space-y-0'>
                    <FormControl>
                      <Switch
                        aria-label={t('Claude Code Client Identity Simulation')}
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
                {t('Claude Code Client Identity Simulation')}
              </FieldLegend>
              <SettingsFormGrid>
                <FormField
                  control={form.control}
                  name='claude.request_header_fallback_version'
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
                  name='claude.request_header_model_patterns'
                  render={({ field }) => (
                    <FormItem>
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
                  {`claude-cli/${identityVersion.trim()} (external, cli)`}
                </code>
              </div>
            </FieldSet>
          </SettingsFormSection>

          <Separator />

          <SettingsFormSection
            title={t('Model Request Configuration')}
            description={t(
              'Configure request headers and default max tokens by model.'
            )}
          >
            <div className='space-y-6'>
              <FormField
                control={form.control}
                name='claude.model_headers_settings'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Request Header Overrides')}</FormLabel>
                    <FormControl>
                      <JsonCodeEditor
                        value={field.value}
                        onChange={field.onChange}
                        name={field.name}
                        onBlur={field.onBlur}
                        textareaRef={field.ref}
                        aria-invalid={Boolean(
                          form.formState.errors.claude?.model_headers_settings
                        )}
                      />
                    </FormControl>
                    <FormDescription>
                      {t(
                        'Provide per-model header overrides as JSON. Useful for enabling beta features such as expanded context windows.'
                      )}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name='claude.default_max_tokens'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Default Max Tokens')}</FormLabel>
                    <FormControl>
                      <JsonCodeEditor
                        value={field.value}
                        onChange={field.onChange}
                        name={field.name}
                        onBlur={field.onBlur}
                        textareaRef={field.ref}
                        aria-invalid={Boolean(
                          form.formState.errors.claude?.default_max_tokens
                        )}
                      />
                    </FormControl>
                    <FormDescription>
                      {t('Example')}{' '}
                      {`{ "default": 8192, "claude-3-haiku-20240307": 4096, "claude-3-opus-20240229": 4096, "claude-3-7-sonnet-20250219-thinking": 8192 }`}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </div>
          </SettingsFormSection>

          <Separator />

          <SettingsFormSection
            title={t('Thinking Suffix Adaptation')}
            description={t(
              'Convert -thinking suffix requests to Anthropic native thinking requests.'
            )}
            action={
              <FormField
                control={form.control}
                name='claude.thinking_adapter_enabled'
                render={({ field }) => (
                  <FormItem className='space-y-0'>
                    <FormControl>
                      <Switch
                        aria-label={t('Thinking Suffix Adaptation')}
                        checked={field.value}
                        onCheckedChange={field.onChange}
                      />
                    </FormControl>
                  </FormItem>
                )}
              />
            }
          >
            <FormField
              control={form.control}
              name='claude.thinking_adapter_budget_tokens_percentage'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Budget Tokens Ratio')}</FormLabel>
                  <FormControl>
                    <Input
                      {...field}
                      value={String(field.value ?? '')}
                      onChange={(event) => field.onChange(event.target.value)}
                    />
                  </FormControl>
                  <FormDescription>
                    {t(
                      'Budget tokens = max tokens × ratio. Accepts a decimal between 0.1 and 1.'
                    )}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
          </SettingsFormSection>
        </SettingsForm>
      </Form>
    </SettingsSection>
  )
}
