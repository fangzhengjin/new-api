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
import { useEffect, useRef } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import * as z from 'zod'

import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormLabel,
} from '@/components/ui/form'
import { Switch } from '@/components/ui/switch'

import {
  SettingsForm,
  SettingsSwitchContent,
  SettingsSwitchItem,
} from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useUpdateOption } from '../hooks/use-update-option'

const schema = z.object({
  codex: z.object({
    request_header_fallback_enabled: z.boolean(),
  }),
})

type CodexSettingsFormValues = z.infer<typeof schema>

type CodexSettingsCardProps = {
  defaultValues: CodexSettingsFormValues
}

/** Renders the global Codex request-header fallback setting. */
export function CodexSettingsCard(props: CodexSettingsCardProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const baselineRef = useRef(
    props.defaultValues.codex.request_header_fallback_enabled
  )
  const form = useForm<CodexSettingsFormValues>({
    resolver: zodResolver(schema),
    defaultValues: props.defaultValues,
  })

  useEffect(() => {
    baselineRef.current =
      props.defaultValues.codex.request_header_fallback_enabled
    form.reset(props.defaultValues)
  }, [props.defaultValues, form])

  const onSubmit = async (values: CodexSettingsFormValues) => {
    const enabled = values.codex.request_header_fallback_enabled
    if (enabled === baselineRef.current) {
      toast.info(t('No changes to save'))
      return
    }

    await updateOption.mutateAsync({
      key: 'codex.request_header_fallback_enabled',
      value: enabled,
    })
    baselineRef.current = enabled
    form.reset(values)
  }

  return (
    <SettingsSection title={t('Codex')}>
      <Form {...form}>
        <SettingsForm onSubmit={form.handleSubmit(onSubmit)}>
          <SettingsPageFormActions
            onSave={form.handleSubmit(onSubmit)}
            isSaving={updateOption.isPending}
          />
          <FormField
            control={form.control}
            name='codex.request_header_fallback_enabled'
            render={({ field }) => (
              <SettingsSwitchItem>
                <SettingsSwitchContent>
                  <FormLabel>{t('Codex Request Header Fallback')}</FormLabel>
                  <FormDescription>
                    {t(
                      'After manual header rules, fill missing Codex TUI identity headers for Responses requests or a missing Codex User-Agent for Chat Completions.'
                    )}
                  </FormDescription>
                </SettingsSwitchContent>
                <FormControl>
                  <Switch
                    aria-label={t('Codex Request Header Fallback')}
                    checked={field.value}
                    onCheckedChange={field.onChange}
                  />
                </FormControl>
              </SettingsSwitchItem>
            )}
          />
        </SettingsForm>
      </Form>
    </SettingsSection>
  )
}
