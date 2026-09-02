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
import { Code2, Palette } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import * as z from 'zod'

import { JsonCodeEditor } from '@/components/json-code-editor'
import { Button } from '@/components/ui/button'
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
import { MODEL_REQUEST_LIMIT_MAX } from '@/lib/request-limits'

import {
  SettingsForm,
  SettingsSwitchContent,
  SettingsSwitchItem,
} from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useUpdateOption } from '../hooks/use-update-option'
import { safeNumberFieldProps } from '../utils/numeric-field'
import {
  createLimitErrorTemplateSchema,
  formatRequestLimitDuration,
} from './limit-error-template'
import {
  LimitErrorTemplateFields,
  type LimitErrorTemplateScenario,
} from './limit-error-template-fields'
import { RateLimitVisualEditor } from './rate-limit-visual-editor'

const maxLimitPeriodMinutes = 1440

const isValidJSON = (value: string | undefined) => {
  if (!value || value.trim() === '') return true
  try {
    const parsed = JSON.parse(value)
    if (typeof parsed !== 'object' || Array.isArray(parsed)) {
      return false
    }
    for (const [, val] of Object.entries(parsed)) {
      if (!Array.isArray(val) || val.length !== 2) return false
      if (typeof val[0] !== 'number' || typeof val[1] !== 'number') return false
      if (val[0] < 0 || val[1] < 0) return false
      if (val[0] > 2147483647 || val[1] > 2147483647) return false
    }
    return true
  } catch {
    return false
  }
}

const createRateLimitSchema = (t: (key: string) => string) =>
  z.object({
    ModelRequestRateLimitEnabled: z.boolean(),
    ModelRequestRateLimitDurationMinutes: z
      .number()
      .int(t('Enter an integer from 1 to 1440'))
      .min(1, t('Enter an integer from 1 to 1440'))
      .max(maxLimitPeriodMinutes, t('Enter an integer from 1 to 1440')),
    ModelRequestRateLimitCount: z
      .number()
      .int(t('Enter an integer from 0 to 100000000'))
      .min(0, t('Enter an integer from 0 to 100000000'))
      .max(MODEL_REQUEST_LIMIT_MAX, t('Enter an integer from 0 to 100000000')),
    ModelRequestIPRateLimitCount: z
      .number()
      .int(t('Enter an integer from 0 to 100000000'))
      .min(0, t('Enter an integer from 0 to 100000000'))
      .max(MODEL_REQUEST_LIMIT_MAX, t('Enter an integer from 0 to 100000000')),
    ModelRequestIPRateLimitSuccessCount: z
      .number()
      .int(t('Enter an integer from 0 to 100000000'))
      .min(0, t('Enter an integer from 0 to 100000000'))
      .max(MODEL_REQUEST_LIMIT_MAX, t('Enter an integer from 0 to 100000000')),
    ModelRequestRateLimitSuccessCount: z
      .number()
      .int(t('Enter an integer from 0 to 100000000'))
      .min(0, t('Enter an integer from 0 to 100000000'))
      .max(MODEL_REQUEST_LIMIT_MAX, t('Enter an integer from 0 to 100000000')),
    ModelRequestRateLimitGroup: z
      .string()
      .optional()
      .refine(isValidJSON, {
        message: t('Invalid JSON format or values out of allowed range'),
      }),
    ModelRequestRateLimitAccountTotalErrorTemplate:
      createLimitErrorTemplateSchema(t),
    ModelRequestRateLimitAccountSuccessErrorTemplate:
      createLimitErrorTemplateSchema(t),
    ModelRequestRateLimitIPTotalErrorTemplate:
      createLimitErrorTemplateSchema(t),
    ModelRequestRateLimitIPSuccessErrorTemplate:
      createLimitErrorTemplateSchema(t),
  })

type RateLimitFormValues = z.infer<ReturnType<typeof createRateLimitSchema>>

type RateLimitSectionProps = {
  defaultValues: RateLimitFormValues
  defaultTemplates: string
}

export function RateLimitSection(props: RateLimitSectionProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const [useVisualEditor, setUseVisualEditor] = useState(true)

  const rateLimitSchema = createRateLimitSchema(t)

  const form = useForm<RateLimitFormValues>({
    resolver: zodResolver(rateLimitSchema),
    mode: 'onChange', // Enable real-time validation
    defaultValues: props.defaultValues,
  })

  useEffect(() => {
    form.reset(props.defaultValues)
  }, [form, props.defaultValues])

  const values = form.watch()
  const period = formatRequestLimitDuration(
    values.ModelRequestRateLimitDurationMinutes * 60
  )
  const retryAfterSeconds = values.ModelRequestRateLimitDurationMinutes * 60
  const errorScenarios: LimitErrorTemplateScenario<RateLimitFormValues>[] = [
    {
      name: 'ModelRequestRateLimitAccountTotalErrorTemplate',
      label: t('Account total requests'),
      code: 'rate_limit_account_total',
      variables: ['Limit', 'Period', 'RetryAfter'],
      samples: {
        Limit: String(values.ModelRequestRateLimitCount),
        Period: period,
        RetryAfter: period,
      },
      retryAfterSeconds,
    },
    {
      name: 'ModelRequestRateLimitAccountSuccessErrorTemplate',
      label: t('Account successful requests'),
      code: 'rate_limit_account_success',
      variables: ['Limit', 'Period', 'RetryAfter'],
      samples: {
        Limit: String(values.ModelRequestRateLimitSuccessCount),
        Period: period,
        RetryAfter: period,
      },
      retryAfterSeconds,
    },
    {
      name: 'ModelRequestRateLimitIPTotalErrorTemplate',
      label: t('IP total requests'),
      code: 'rate_limit_ip_total',
      variables: ['Limit', 'Period', 'RetryAfter'],
      samples: {
        Limit: String(values.ModelRequestIPRateLimitCount),
        Period: period,
        RetryAfter: period,
      },
      retryAfterSeconds,
    },
    {
      name: 'ModelRequestRateLimitIPSuccessErrorTemplate',
      label: t('IP successful requests'),
      code: 'rate_limit_ip_success',
      variables: ['Limit', 'Period', 'RetryAfter'],
      samples: {
        Limit: String(values.ModelRequestIPRateLimitSuccessCount),
        Period: period,
        RetryAfter: period,
      },
      retryAfterSeconds,
    },
  ]

  const onSubmit = async (values: RateLimitFormValues) => {
    const updates = Object.entries(values).filter(
      ([key, value]) =>
        value !== props.defaultValues[key as keyof RateLimitFormValues]
    )

    if (updates.length > 0) {
      await updateOption.mutateAsync(
        updates.map(([key, value]) => ({ key, value: value ?? '' }))
      )
    }
  }

  return (
    <SettingsSection title={t('Rate Limiting')}>
      <Form {...form}>
        <SettingsForm onSubmit={form.handleSubmit(onSubmit)}>
          <SettingsPageFormActions
            onSave={form.handleSubmit(onSubmit)}
            isSaving={updateOption.isPending}
            saveLabel={t('Save rate limits')}
          />
          <FormField
            control={form.control}
            name='ModelRequestRateLimitEnabled'
            render={({ field }) => (
              <SettingsSwitchItem>
                <SettingsSwitchContent>
                  <FormLabel>{t('Enable rate limiting')}</FormLabel>
                  <FormDescription>
                    {t(
                      'This controls model request rate limiting. Web/API route throttling is configured by environment variables and may still return 429.'
                    )}
                  </FormDescription>
                </SettingsSwitchContent>
                <FormControl>
                  <Switch
                    checked={field.value}
                    onCheckedChange={field.onChange}
                  />
                </FormControl>
              </SettingsSwitchItem>
            )}
          />

          <div className='space-y-4'>
            <FormField
              control={form.control}
              name='ModelRequestRateLimitDurationMinutes'
              render={({ field }) => (
                <FormItem className='md:max-w-[calc(50%_-_0.5rem)]'>
                  <FormLabel>{t('Limit period (minutes)')}</FormLabel>
                  <FormControl>
                    <Input
                      type='number'
                      min={1}
                      max={maxLimitPeriodMinutes}
                      step={1}
                      {...safeNumberFieldProps(field)}
                    />
                  </FormControl>
                  <FormDescription>
                    {t('Account and IP limits share this period')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <fieldset className='grid gap-4 md:grid-cols-2'>
              <legend className='sr-only'>{t('Account Request Limits')}</legend>
              <FormField
                control={form.control}
                name='ModelRequestRateLimitCount'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>
                      {t('Max requests per account per period')}
                    </FormLabel>
                    <FormControl>
                      <Input
                        type='number'
                        min={0}
                        max={MODEL_REQUEST_LIMIT_MAX}
                        step={1}
                        {...safeNumberFieldProps(field)}
                      />
                    </FormControl>
                    <FormDescription>
                      {t(
                        'Counts all requests from the same account, including failures, 0 = unlimited'
                      )}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name='ModelRequestRateLimitSuccessCount'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>
                      {t('Max successful requests per account per period')}
                    </FormLabel>
                    <FormControl>
                      <Input
                        type='number'
                        min={0}
                        max={MODEL_REQUEST_LIMIT_MAX}
                        step={1}
                        {...safeNumberFieldProps(field)}
                      />
                    </FormControl>
                    <FormDescription>
                      {t(
                        'Counts only successful requests from the same account, 0 = unlimited'
                      )}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </fieldset>

            <fieldset className='grid gap-4 md:grid-cols-2'>
              <legend className='sr-only'>{t('IP Request Limits')}</legend>
              <FormField
                control={form.control}
                name='ModelRequestIPRateLimitCount'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Max requests per IP per period')}</FormLabel>
                    <FormControl>
                      <Input
                        type='number'
                        min={0}
                        max={MODEL_REQUEST_LIMIT_MAX}
                        step={1}
                        {...safeNumberFieldProps(field)}
                      />
                    </FormControl>
                    <FormDescription>
                      {t(
                        'Counts all requests from the same IP, including failures, 0 = unlimited'
                      )}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name='ModelRequestIPRateLimitSuccessCount'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>
                      {t('Max successful requests per IP per period')}
                    </FormLabel>
                    <FormControl>
                      <Input
                        type='number'
                        min={0}
                        max={MODEL_REQUEST_LIMIT_MAX}
                        step={1}
                        {...safeNumberFieldProps(field)}
                      />
                    </FormControl>
                    <FormDescription>
                      {t(
                        'Counts only successful requests from the same IP, 0 = unlimited'
                      )}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </fieldset>
          </div>

          <p className='text-muted-foreground text-sm'>
            {t(
              'When multiple limits are reached, checks run in this order: account requests, account successful requests, IP requests, IP successful requests'
            )}
          </p>

          <LimitErrorTemplateFields
            form={form}
            defaultTemplates={props.defaultTemplates}
            scenarios={errorScenarios}
          />

          <FormField
            control={form.control}
            name='ModelRequestRateLimitGroup'
            render={({ field }) => (
              <FormItem>
                <div className='flex items-center justify-between'>
                  <FormLabel>{t('Group-based rate limits')}</FormLabel>
                  <Button
                    type='button'
                    variant='outline'
                    size='sm'
                    onClick={() => setUseVisualEditor(!useVisualEditor)}
                  >
                    {useVisualEditor ? (
                      <>
                        <Code2 className='mr-2 h-4 w-4' />
                        {t('JSON Mode')}
                      </>
                    ) : (
                      <>
                        <Palette className='mr-2 h-4 w-4' />
                        {t('Visual Mode')}
                      </>
                    )}
                  </Button>
                </div>
                <FormControl>
                  {useVisualEditor ? (
                    <RateLimitVisualEditor
                      value={field.value || ''}
                      onChange={field.onChange}
                    />
                  ) : (
                    <JsonCodeEditor
                      value={field.value || ''}
                      onChange={field.onChange}
                      name={field.name}
                      onBlur={field.onBlur}
                      textareaRef={field.ref}
                      placeholder={`{\n  "default": [200, 100],\n  "vip": [0, 1000]\n}`}
                      aria-invalid={Boolean(
                        form.formState.errors.ModelRequestRateLimitGroup
                      )}
                    />
                  )}
                </FormControl>
                <FormDescription>
                  {useVisualEditor ? (
                    t(
                      'Only overrides account request limits, does not affect IP limits, and accounts in the same group do not share counters'
                    )
                  ) : (
                    <div className='space-y-1 text-xs'>
                      <p className='font-semibold'>{t('Format:')}</p>
                      <ul className='list-inside list-disc space-y-0.5 pl-2'>
                        <li>
                          {t('JSON object:')}{' '}
                          {`{"groupName": [maxRequests, maxSuccess]}`}
                        </li>
                        <li>
                          {t('Example:')}{' '}
                          {`{"default": [200, 100], "vip": [0, 1000]}`}
                        </li>
                        <li>
                          {t(
                            'maxRequests ≥ 0, maxSuccess ≥ 0, both ≤ 2,147,483,647'
                          )}
                        </li>
                        <li>
                          {t(
                            'Only overrides account request limits, does not affect IP limits, and accounts in the same group do not share counters'
                          )}
                        </li>
                      </ul>
                    </div>
                  )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />
        </SettingsForm>
      </Form>
    </SettingsSection>
  )
}
