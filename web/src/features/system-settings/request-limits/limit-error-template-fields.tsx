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
import { useMemo, useRef, useState } from 'react'
import type { FieldValues, Path, UseFormReturn } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { Accordion } from '@/components/ui/accordion'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Textarea } from '@/components/ui/textarea'

import { SettingsAccordion } from '../components/settings-accordion'

type TemplateVariable = 'Limit' | 'Period' | 'RetryAfter'

export type LimitErrorTemplateScenario<TValues extends FieldValues> = {
  name: Path<TValues>
  label: string
  code: string
  variables: readonly TemplateVariable[]
  samples: Partial<Record<TemplateVariable, string>>
  retryAfterSeconds?: number
}

export function LimitErrorTemplateFields<TValues extends FieldValues>(props: {
  form: UseFormReturn<TValues>
  defaultTemplates: string
  scenarios: readonly LimitErrorTemplateScenario<TValues>[]
}) {
  const { t } = useTranslation()
  const [activeName, setActiveName] = useState(props.scenarios[0].name)
  const textareaRef = useRef<HTMLTextAreaElement | null>(null)
  const activeScenario =
    props.scenarios.find((scenario) => scenario.name === activeName) ??
    props.scenarios[0]
  const defaults = useMemo(() => {
    try {
      const parsed = JSON.parse(props.defaultTemplates) as unknown
      if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
        return {}
      }
      return Object.fromEntries(
        Object.entries(parsed).filter((entry) => typeof entry[1] === 'string')
      ) as Record<string, string>
    } catch {
      return {}
    }
  }, [props.defaultTemplates])

  const variableDescriptions: Record<TemplateVariable, string> = {
    Limit: t('Triggered limit value'),
    Period: t('Limit period'),
    RetryAfter: t('Suggested wait time'),
  }

  return (
    <Accordion
      defaultValue={['rejection-messages']}
      className='rounded-xl border px-4'
    >
      <SettingsAccordion
        value='rejection-messages'
        title={t('Rejection messages')}
        className='border-b-0'
      >
        <p className='text-muted-foreground mb-4 text-sm'>
          {t(
            'Customize the bilingual message returned when a request limit is reached'
          )}
        </p>

        <div className='mb-4 space-y-2'>
          <div className='text-sm font-medium'>{t('Rejection scenario')}</div>
          <div
            className='flex flex-wrap gap-2'
            role='group'
            aria-label={t('Rejection scenario')}
          >
            {props.scenarios.map((scenario) => (
              <Button
                key={scenario.name}
                type='button'
                size='sm'
                variant={
                  scenario.name === activeScenario.name
                    ? 'secondary'
                    : 'outline'
                }
                aria-pressed={scenario.name === activeScenario.name}
                onClick={() => setActiveName(scenario.name)}
              >
                {scenario.label}
              </Button>
            ))}
          </div>
        </div>

        <FormField
          key={activeScenario.name}
          control={props.form.control}
          name={activeScenario.name}
          render={({ field }) => {
            const value = String(field.value ?? '')
            const usesDefault = value.trim() === ''
            const source = usesDefault
              ? defaults[activeScenario.name] ||
                t('Default preview unavailable')
              : value
            let preview = source
            for (const variable of activeScenario.variables) {
              preview = preview.replaceAll(
                `{{.${variable}}}`,
                activeScenario.samples[variable] ?? ''
              )
            }

            const insertVariable = (variable: TemplateVariable) => {
              const token = `{{.${variable}}}`
              const textarea = textareaRef.current
              if (!textarea) {
                field.onChange(value + token)
                return
              }
              const start = textarea.selectionStart
              const end = textarea.selectionEnd
              const nextValue = value.slice(0, start) + token + value.slice(end)
              field.onChange(nextValue)
              textarea.focus()
              textarea.setSelectionRange(
                start + token.length,
                start + token.length
              )
            }

            return (
              <div className='grid gap-4 lg:grid-cols-[minmax(0,1.1fr)_minmax(18rem,0.9fr)]'>
                <FormItem>
                  <FormLabel>
                    {t('Custom rejection message (optional)')}
                  </FormLabel>
                  <FormControl>
                    <Textarea
                      {...field}
                      ref={(node) => {
                        textareaRef.current = node
                        field.ref(node)
                      }}
                      value={value}
                      className='min-h-40 font-mono text-sm'
                      placeholder={t(
                        'Leave blank to use the built-in bilingual message'
                      )}
                    />
                  </FormControl>
                  <FormMessage />

                  <div className='space-y-2 pt-2'>
                    <div className='text-sm font-medium'>
                      {t('Available variables')}
                    </div>
                    <div className='grid gap-2 sm:grid-cols-2'>
                      {activeScenario.variables.map((variable) => (
                        <Button
                          key={variable}
                          type='button'
                          size='sm'
                          variant='outline'
                          className='h-auto min-w-0 justify-start gap-2 py-2'
                          aria-label={t('Insert variable {{variable}}', {
                            variable: `{{.${variable}}}`,
                          })}
                          onClick={() => insertVariable(variable)}
                        >
                          <code className='bg-muted rounded px-1.5 py-0.5 text-xs'>
                            {`{{.${variable}}}`}
                          </code>
                          <span className='text-muted-foreground truncate text-xs'>
                            {variableDescriptions[variable]}
                          </span>
                        </Button>
                      ))}
                    </div>
                  </div>

                  <div className='flex flex-wrap items-center justify-between gap-2 pt-2'>
                    <FormDescription className='space-y-1'>
                      <span className='block'>
                        {t(
                          'Wait times use formats such as 45s, 2m 15s, and 1h 5m'
                        )}
                      </span>
                      <span className='block'>
                        {t(
                          'English and Chinese are returned on separate lines, variables use {{.Variable}}'
                        )}
                      </span>
                    </FormDescription>
                    <Button
                      type='button'
                      variant='link'
                      size='sm'
                      onClick={() =>
                        field.onChange(defaults[activeScenario.name] ?? '')
                      }
                    >
                      {t('Use default as starting point')}
                    </Button>
                  </div>
                </FormItem>

                <aside
                  className='bg-muted/30 rounded-xl border p-4'
                  aria-live='polite'
                >
                  <div className='mb-3 flex items-center justify-between gap-2'>
                    <span className='text-sm font-medium'>{t('Preview')}</span>
                    <Badge variant='secondary'>
                      {usesDefault ? t('System default') : t('Custom')}
                    </Badge>
                  </div>
                  <pre className='bg-background min-h-24 rounded-lg border p-3 font-sans text-sm leading-relaxed break-words whitespace-pre-wrap'>
                    {preview}
                  </pre>
                  <dl className='text-muted-foreground mt-3 space-y-2 text-xs'>
                    <div className='flex items-center justify-between gap-4'>
                      <dt>error.code</dt>
                      <dd>
                        <code className='bg-muted rounded px-1.5 py-0.5'>
                          {activeScenario.code}
                        </code>
                      </dd>
                    </div>
                    {activeScenario.retryAfterSeconds !== undefined && (
                      <div className='flex items-center justify-between gap-4'>
                        <dt>Retry-After</dt>
                        <dd>
                          <code className='bg-muted rounded px-1.5 py-0.5'>
                            {activeScenario.retryAfterSeconds}
                          </code>
                        </dd>
                      </div>
                    )}
                  </dl>
                  <p className='text-muted-foreground mt-3 border-t pt-3 text-xs leading-relaxed'>
                    {t(
                      'Internal cache or storage failures continue to use fixed system messages'
                    )}
                  </p>
                </aside>
              </div>
            )
          }}
        />
      </SettingsAccordion>
    </Accordion>
  )
}
