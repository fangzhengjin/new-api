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
import { useEffect } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import * as z from 'zod'

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
import { MODEL_REQUEST_CONCURRENCY_MAX } from '@/lib/request-limits'

import {
  SettingsForm,
  SettingsSwitchContent,
  SettingsSwitchItem,
} from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useUpdateOption } from '../hooks/use-update-option'
import { safeNumberFieldProps } from '../utils/numeric-field'
import { createLimitErrorTemplateSchema } from './limit-error-template'
import {
  LimitErrorTemplateFields,
  type LimitErrorTemplateScenario,
} from './limit-error-template-fields'

const createConcurrencyLimitSchema = (t: (key: string) => string) =>
  z.object({
    ModelRequestConcurrencyLimitEnabled: z.boolean(),
    ModelRequestConcurrencyLimit: z
      .number()
      .int(t('Enter an integer from 0 to 10000'))
      .min(0, t('Enter an integer from 0 to 10000'))
      .max(
        MODEL_REQUEST_CONCURRENCY_MAX,
        t('Enter an integer from 0 to 10000')
      ),
    ModelRequestIPConcurrencyLimit: z
      .number()
      .int(t('Enter an integer from 0 to 10000'))
      .min(0, t('Enter an integer from 0 to 10000'))
      .max(
        MODEL_REQUEST_CONCURRENCY_MAX,
        t('Enter an integer from 0 to 10000')
      ),
    ModelRequestConcurrencyAccountErrorTemplate:
      createLimitErrorTemplateSchema(t),
    ModelRequestConcurrencyIPErrorTemplate: createLimitErrorTemplateSchema(t),
  })

type ConcurrencyLimitFormValues = z.infer<
  ReturnType<typeof createConcurrencyLimitSchema>
>

export function ConcurrencyLimitSection(props: {
  defaultValues: ConcurrencyLimitFormValues
  defaultTemplates: string
}) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const form = useForm<ConcurrencyLimitFormValues>({
    resolver: zodResolver(createConcurrencyLimitSchema(t)),
    mode: 'onChange',
    defaultValues: props.defaultValues,
  })

  useEffect(() => {
    form.reset(props.defaultValues)
  }, [form, props.defaultValues])

  const values = form.watch()
  const errorScenarios: LimitErrorTemplateScenario<ConcurrencyLimitFormValues>[] =
    [
      {
        name: 'ModelRequestConcurrencyAccountErrorTemplate',
        label: t('Account concurrent requests'),
        code: 'account_concurrency_limit',
        variables: ['Limit'],
        samples: { Limit: String(values.ModelRequestConcurrencyLimit) },
      },
      {
        name: 'ModelRequestConcurrencyIPErrorTemplate',
        label: t('IP concurrent requests'),
        code: 'ip_concurrency_limit',
        variables: ['Limit'],
        samples: { Limit: String(values.ModelRequestIPConcurrencyLimit) },
      },
    ]

  const onSubmit = async (values: ConcurrencyLimitFormValues) => {
    const updates = Object.entries(values).filter(
      ([key, value]) =>
        value !== props.defaultValues[key as keyof ConcurrencyLimitFormValues]
    )
    if (updates.length > 0) {
      await updateOption.mutateAsync(
        updates.map(([key, value]) => ({ key, value }))
      )
    }
  }

  return (
    <SettingsSection title={t('Concurrency Limits')}>
      <Form {...form}>
        <SettingsForm onSubmit={form.handleSubmit(onSubmit)}>
          <SettingsPageFormActions
            onSave={form.handleSubmit(onSubmit)}
            isSaving={updateOption.isPending}
            saveLabel={t('Save concurrency limits')}
          />

          <FormField
            control={form.control}
            name='ModelRequestConcurrencyLimitEnabled'
            render={({ field }) => (
              <SettingsSwitchItem>
                <SettingsSwitchContent>
                  <FormLabel>{t('Enable concurrency limits')}</FormLabel>
                  <FormDescription>
                    {t('Limit concurrent model requests by account and IP')}
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

          <fieldset className='grid gap-4 md:grid-cols-2'>
            <legend className='sr-only'>{t('Concurrency Limits')}</legend>
            <FormField
              control={form.control}
              name='ModelRequestConcurrencyLimit'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>
                    {t('Max concurrent requests per account')}
                  </FormLabel>
                  <FormControl>
                    <Input
                      type='number'
                      min={0}
                      max={MODEL_REQUEST_CONCURRENCY_MAX}
                      step={1}
                      {...safeNumberFieldProps(field)}
                    />
                  </FormControl>
                  <FormDescription>
                    {t(
                      'Counts requests currently being processed, 0 = unlimited'
                    )}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name='ModelRequestIPConcurrencyLimit'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Max concurrent requests per IP')}</FormLabel>
                  <FormControl>
                    <Input
                      type='number'
                      min={0}
                      max={MODEL_REQUEST_CONCURRENCY_MAX}
                      step={1}
                      {...safeNumberFieldProps(field)}
                    />
                  </FormControl>
                  <FormDescription>
                    {t(
                      'Counts requests currently being processed, 0 = unlimited'
                    )}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
          </fieldset>

          <p className='text-muted-foreground text-sm'>
            {t(
              'Checked before period rate limits: account concurrency first, then IP concurrency'
            )}
          </p>

          <LimitErrorTemplateFields
            form={form}
            defaultTemplates={props.defaultTemplates}
            scenarios={errorScenarios}
          />
        </SettingsForm>
      </Form>
    </SettingsSection>
  )
}
