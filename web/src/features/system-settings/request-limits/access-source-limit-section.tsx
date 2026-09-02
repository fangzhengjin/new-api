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
      .int(t('Enter an integer from 1 to 168'))
      .min(1, t('Enter an integer from 1 to 168'))
      .max(
        ACCESS_SOURCE_ASSOCIATION_WINDOW_HOURS_MAX,
        t('Enter an integer from 1 to 168')
      ),
    AccessSourceMaxIPsPerUser: z
      .number()
      .int(t('Enter an integer from 0 to 1000'))
      .min(0, t('Enter an integer from 0 to 1000'))
      .max(ACCESS_SOURCE_ASSOCIATION_MAX, t('Enter an integer from 0 to 1000')),
    AccessSourceSwitchCooldownMinutes: z
      .number()
      .int(t('Enter an integer from 0 to 1440'))
      .min(0, t('Enter an integer from 0 to 1440'))
      .max(
        ACCESS_SOURCE_SWITCH_COOLDOWN_MINUTES_MAX,
        t('Enter an integer from 0 to 1440')
      ),
    AccessSourceMaxUsersPerIP: z
      .number()
      .int(t('Enter an integer from 0 to 1000'))
      .min(0, t('Enter an integer from 0 to 1000'))
      .max(ACCESS_SOURCE_ASSOCIATION_MAX, t('Enter an integer from 0 to 1000')),
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
        label: t('IP switch wait'),
        code: 'access_source_switch_cooldown',
        variables: ['RetryAfter'],
        samples: {
          RetryAfter: formatRequestLimitDuration(retryAfterSeconds),
        },
        retryAfterSeconds,
      },
      {
        name: 'AccessSourceAccountIPLimitErrorTemplate',
        label: t('Account associated IPs'),
        code: 'access_source_account_ip_limit',
        variables: ['Limit', 'Period'],
        samples: {
          Limit: String(values.AccessSourceMaxIPsPerUser),
          Period: period,
        },
      },
      {
        name: 'AccessSourceIPAccountLimitErrorTemplate',
        label: t('IP associated accounts'),
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
      await updateOption.mutateAsync(
        updates.map(([key, value]) => ({ key, value }))
      )
    }
  }

  return (
    <SettingsSection title={t('Access Source Limits')}>
      <Form {...form}>
        <SettingsForm onSubmit={form.handleSubmit(onSubmit)}>
          <SettingsPageFormActions
            onSave={form.handleSubmit(onSubmit)}
            isSaving={updateOption.isPending}
            saveLabel={t('Save access source limits')}
          />
          <FormField
            control={form.control}
            name='AccessSourceLimitEnabled'
            render={({ field }) => (
              <SettingsSwitchItem>
                <SettingsSwitchContent>
                  <FormLabel>{t('Enable access source limits')}</FormLabel>
                  <FormDescription>
                    {t(
                      'Limits one account using multiple IPs, one IP using multiple accounts, and frequent IP switching'
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

          <div className='grid gap-4 md:grid-cols-2'>
            <FormField
              control={form.control}
              name='AccessSourceAssociationWindowHours'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Association period (hours)')}</FormLabel>
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
                    {t(
                      'Counts associations active during the last 1–168 hours'
                    )}
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
                  <FormLabel>{t('IP switch wait (minutes)')}</FormLabel>
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
                    {t(
                      'A different IP can be used after the current IP has been inactive for this many minutes, 0 = no wait'
                    )}
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
                  <FormLabel>{t('Max IPs per account')}</FormLabel>
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
                    {t(
                      'Maximum active IP associations per account, 0 = unlimited'
                    )}
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
                  <FormLabel>{t('Max accounts per IP')}</FormLabel>
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
                    {t(
                      'Maximum active account associations per IP, 0 = unlimited'
                    )}
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
