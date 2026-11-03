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
      .int(t('请输入 0 到 10000 之间的整数'))
      .min(0, t('请输入 0 到 10000 之间的整数'))
      .max(
        MODEL_REQUEST_CONCURRENCY_MAX,
        t('请输入 0 到 10000 之间的整数')
      ),
    ModelRequestIPConcurrencyLimit: z
      .number()
      .int(t('请输入 0 到 10000 之间的整数'))
      .min(0, t('请输入 0 到 10000 之间的整数'))
      .max(
        MODEL_REQUEST_CONCURRENCY_MAX,
        t('请输入 0 到 10000 之间的整数')
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
        label: t('账号并发请求数'),
        code: 'account_concurrency_limit',
        variables: ['Limit'],
        samples: { Limit: String(values.ModelRequestConcurrencyLimit) },
      },
      {
        name: 'ModelRequestConcurrencyIPErrorTemplate',
        label: t('IP 并发请求数'),
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
      for (const [key, value] of updates) {
        await updateOption.mutateAsync({ key, value })
      }
    }
  }

  return (
    <SettingsSection title={t('并发限制')}>
      <Form {...form}>
        <SettingsForm onSubmit={form.handleSubmit(onSubmit)}>
          <SettingsPageFormActions
            onSave={form.handleSubmit(onSubmit)}
            isSaving={updateOption.isPending}
            saveLabel={t('保存并发限制')}
          />

          <FormField
            control={form.control}
            name='ModelRequestConcurrencyLimitEnabled'
            render={({ field }) => (
              <SettingsSwitchItem>
                <SettingsSwitchContent>
                  <FormLabel>{t('启用并发限制')}</FormLabel>
                  <FormDescription>
                    {t('按账户和 IP 限制模型并发请求数')}
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
            <legend className='sr-only'>{t('并发限制')}</legend>
            <FormField
              control={form.control}
              name='ModelRequestConcurrencyLimit'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>
                    {t('每账户最大并发请求数')}
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
                    {t('统计正在处理的请求，0 表示不限制')}
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
                  <FormLabel>{t('每 IP 最大并发请求数')}</FormLabel>
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
                    {t('统计正在处理的请求，0 表示不限制')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
          </fieldset>

          <p className='text-muted-foreground text-sm'>
            {t('先于周期速率限制检查，先检查账户并发，再检查 IP 并发')}
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
