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
import {
  ACCESS_SOURCE_ASSOCIATION_MAX,
  ACCESS_SOURCE_ASSOCIATION_WINDOW_HOURS_MAX,
  ACCESS_SOURCE_SWITCH_COOLDOWN_MINUTES_MAX,
} from '@/lib/request-limits'

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

const createSchema = (t: (key: string) => string) =>
  z.object({
    AccessSourceLimitEnabled: z.boolean(),
    AccessSourceAssociationWindowHours: z
      .number()
      .int(t('请输入 1 至 168 的整数'))
      .min(1, t('请输入 1 至 168 的整数'))
      .max(
        ACCESS_SOURCE_ASSOCIATION_WINDOW_HOURS_MAX,
        t('请输入 1 至 168 的整数')
      ),
    AccessSourceMaxIPsPerUser: z
      .number()
      .int(t('请输入 0 至 1000 的整数'))
      .min(0, t('请输入 0 至 1000 的整数'))
      .max(ACCESS_SOURCE_ASSOCIATION_MAX, t('请输入 0 至 1000 的整数')),
    AccessSourceSwitchCooldownMinutes: z
      .number()
      .int(t('请输入 0 至 1440 的整数'))
      .min(0, t('请输入 0 至 1440 的整数'))
      .max(
        ACCESS_SOURCE_SWITCH_COOLDOWN_MINUTES_MAX,
        t('请输入 0 至 1440 的整数')
      ),
    AccessSourceMaxUsersPerIP: z
      .number()
      .int(t('请输入 0 至 1000 的整数'))
      .min(0, t('请输入 0 至 1000 的整数'))
      .max(ACCESS_SOURCE_ASSOCIATION_MAX, t('请输入 0 至 1000 的整数')),
    AccessSourceSwitchCooldownErrorTemplate: createLimitErrorTemplateSchema(t),
    AccessSourceAccountIPLimitErrorTemplate: createLimitErrorTemplateSchema(t),
    AccessSourceIPAccountLimitErrorTemplate: createLimitErrorTemplateSchema(t),
  })

type AccessSourceLimitValues = z.infer<ReturnType<typeof createSchema>>

export function AccessSourceLimitSection(props: {
  defaultValues: AccessSourceLimitValues
  defaultTemplates: string
}) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const form = useForm<AccessSourceLimitValues>({
    resolver: zodResolver(createSchema(t)),
    mode: 'onChange',
    defaultValues: props.defaultValues,
  })

  useEffect(() => {
    form.reset(props.defaultValues)
  }, [form, props.defaultValues])

  const values = form.watch()
  const period = formatRequestLimitDuration(
    values.AccessSourceAssociationWindowHours * 60 * 60
  )
  const retryAfterSeconds = values.AccessSourceSwitchCooldownMinutes * 60
  const errorScenarios: LimitErrorTemplateScenario<AccessSourceLimitValues>[] =
    [
      {
        name: 'AccessSourceSwitchCooldownErrorTemplate',
        label: t('IP 切换等待'),
        code: 'access_source_switch_cooldown',
        variables: ['RetryAfter'],
        samples: {
          RetryAfter: formatRequestLimitDuration(retryAfterSeconds),
        },
        retryAfterSeconds,
      },
      {
        name: 'AccessSourceAccountIPLimitErrorTemplate',
        label: t('账号关联 IP 数'),
        code: 'access_source_account_ip_limit',
        variables: ['Limit', 'Period'],
        samples: {
          Limit: String(values.AccessSourceMaxIPsPerUser),
          Period: period,
        },
      },
      {
        name: 'AccessSourceIPAccountLimitErrorTemplate',
        label: t('IP 关联账号数'),
        code: 'access_source_ip_account_limit',
        variables: ['Limit', 'Period'],
        samples: {
          Limit: String(values.AccessSourceMaxUsersPerIP),
          Period: period,
        },
      },
    ]

  const onSubmit = async (values: AccessSourceLimitValues) => {
    const updates = Object.entries(values).filter(
      ([key, value]) =>
        value !== props.defaultValues[key as keyof AccessSourceLimitValues]
    )
    if (updates.length > 0) {
      for (const [key, value] of updates) {
        await updateOption.mutateAsync({ key, value })
      }
    }
  }

  return (
    <SettingsSection title={t('访问来源限制')}>
      <Form {...form}>
        <SettingsForm onSubmit={form.handleSubmit(onSubmit)}>
          <SettingsPageFormActions
            onSave={form.handleSubmit(onSubmit)}
            isSaving={updateOption.isPending}
            saveLabel={t('保存访问来源限制')}
          />
          <FormField
            control={form.control}
            name='AccessSourceLimitEnabled'
            render={({ field }) => (
              <SettingsSwitchItem>
                <SettingsSwitchContent>
                  <FormLabel>{t('启用访问来源限制')}</FormLabel>
                  <FormDescription>
                    {t('限制同一账户使用多个 IP、同一 IP 使用多个账户以及频繁切换 IP')}
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

          <div className='grid gap-4 md:grid-cols-2'>
            <FormField
              control={form.control}
              name='AccessSourceAssociationWindowHours'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('关联统计周期（小时）')}</FormLabel>
                  <FormControl>
                    <Input
                      type='number'
                      min={1}
                      max={ACCESS_SOURCE_ASSOCIATION_WINDOW_HOURS_MAX}
                      step={1}
                      {...safeNumberFieldProps(field)}
                    />
                  </FormControl>
                  <FormDescription>
                    {t('统计最近 1–168 小时内活跃的关联关系')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name='AccessSourceSwitchCooldownMinutes'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('IP 切换等待时间（分钟）')}</FormLabel>
                  <FormControl>
                    <Input
                      type='number'
                      min={0}
                      max={ACCESS_SOURCE_SWITCH_COOLDOWN_MINUTES_MAX}
                      step={1}
                      {...safeNumberFieldProps(field)}
                    />
                  </FormControl>
                  <FormDescription>
                    {t('当前 IP 停止活动达到此时长后才可使用其他 IP，0 表示不等待')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name='AccessSourceMaxIPsPerUser'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('每账户最大 IP 数')}</FormLabel>
                  <FormControl>
                    <Input
                      type='number'
                      min={0}
                      max={ACCESS_SOURCE_ASSOCIATION_MAX}
                      step={1}
                      {...safeNumberFieldProps(field)}
                    />
                  </FormControl>
                  <FormDescription>
                    {t('同一账户可关联的最大活跃 IP 数，0 表示不限制')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name='AccessSourceMaxUsersPerIP'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('每 IP 最大账户数')}</FormLabel>
                  <FormControl>
                    <Input
                      type='number'
                      min={0}
                      max={ACCESS_SOURCE_ASSOCIATION_MAX}
                      step={1}
                      {...safeNumberFieldProps(field)}
                    />
                  </FormControl>
                  <FormDescription>
                    {t('同一 IP 可关联的最大活跃账户数，0 表示不限制')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
          </div>

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
