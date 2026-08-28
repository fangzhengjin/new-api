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
/* eslint-disable react/no-array-index-key -- Persisted ordered rules and message patterns have no separate identifiers; all inputs are controlled. */
import { Code2, Eye, FileText, Info, Plus, Trash2 } from 'lucide-react'
import { useId, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { JsonCodeEditor } from '@/components/json-code-editor'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardAction,
  CardContent,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { FieldError } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'

import { SettingsFormSection } from '../components/settings-form-layout'
import {
  createEmptyCodexErrorResponseMapping,
  getCodexErrorResponseMappingIssues,
  parseCodexErrorResponseMappingDrafts,
  retryableRateLimitTemplate,
  validateCodexErrorResponseMappings,
  type CodexErrorResponseMapping,
} from './codex-error-response-mapping'

type CodexErrorResponseMappingsProps = {
  value: string
  onChange: (value: string) => void
  onBlur?: () => void
  invalid?: boolean
}

/** Edits ordered Codex response mappings in visual or JSON form. */
export function CodexErrorResponseMappings(
  props: CodexErrorResponseMappingsProps
) {
  const { t } = useTranslation()
  const fieldId = useId()
  const [mode, setMode] = useState<'visual' | 'json'>('visual')
  const rules = useMemo(
    () => parseCodexErrorResponseMappingDrafts(props.value),
    [props.value]
  )
  const validation = useMemo(
    () => validateCodexErrorResponseMappings(props.value),
    [props.value]
  )
  const ruleCount = rules?.length ?? 0
  const statusLabel =
    ruleCount === 1
      ? t('Configured · 1 rule')
      : t('Configured · {{count}} rules', { count: ruleCount })

  const syncRules = (nextRules: CodexErrorResponseMapping[]) => {
    props.onChange(JSON.stringify(nextRules, null, 2))
  }

  const updateRule = (index: number, nextRule: CodexErrorResponseMapping) => {
    if (!rules) return
    const nextRules = [...rules]
    nextRules[index] = nextRule
    syncRules(nextRules)
  }

  const addRule = () => {
    if (!rules) return
    syncRules([...rules, createEmptyCodexErrorResponseMapping()])
    setMode('visual')
  }

  const fillTemplate = () => {
    if (!rules) return
    syncRules([...rules, structuredClone(retryableRateLimitTemplate)])
    setMode('visual')
  }

  const toggleMode = () => {
    if (mode === 'json' && !rules) return
    setMode((current) => (current === 'visual' ? 'json' : 'visual'))
  }

  const actions = (
    <div
      role='group'
      aria-label={t('Error response mapping actions')}
      className='flex flex-wrap justify-end gap-2'
    >
      <Button
        type='button'
        variant='outline'
        size='sm'
        className='w-7 px-0 sm:w-auto sm:px-2.5'
        onClick={toggleMode}
        disabled={mode === 'json' && !rules}
        aria-label={
          mode === 'visual' ? t('Switch to JSON') : t('Switch to Visual')
        }
      >
        {mode === 'visual' ? (
          <Code2 data-icon='inline-start' aria-hidden='true' />
        ) : (
          <Eye data-icon='inline-start' aria-hidden='true' />
        )}
        <span className='hidden sm:inline'>
          {mode === 'visual' ? t('JSON') : t('Visual')}
        </span>
      </Button>
      <Button
        type='button'
        variant='outline'
        size='sm'
        className='w-7 px-0 sm:w-auto sm:px-2.5'
        onClick={fillTemplate}
        disabled={!rules}
        aria-label={t('Fill template')}
      >
        <FileText data-icon='inline-start' aria-hidden='true' />
        <span className='hidden sm:inline'>{t('Fill template')}</span>
      </Button>
      <Button
        type='button'
        size='sm'
        className='w-7 px-0 sm:w-auto sm:px-2.5'
        onClick={addRule}
        disabled={!rules}
        aria-label={t('Add mapping rule')}
      >
        <Plus data-icon='inline-start' aria-hidden='true' />
        <span className='hidden sm:inline'>{t('Add rule')}</span>
      </Button>
    </div>
  )

  return (
    <SettingsFormSection
      title={t('Codex Error Response Mappings')}
      description={t(
        'Rewrite selected upstream errors for Codex clients, while an empty configuration leaves every response unchanged'
      )}
      action={actions}
      className='pb-10 sm:pb-14 [&>div:first-child>div:first-child]:flex-1'
    >
      <Alert role='note'>
        <Info aria-hidden='true' />
        <AlertTitle>{t('Fixed scope')}</AlertTitle>
        <AlertDescription>
          {t(
            'Only requests whose User-Agent is recognized as Codex and whose path is exactly /v1/responses are processed, and message matching ignores case'
          )}
        </AlertDescription>
      </Alert>

      <div className='flex flex-wrap items-center gap-2'>
        <Badge variant='secondary'>{statusLabel}</Badge>
        <span className='text-muted-foreground text-xs'>
          {t('Rules are evaluated in order, and the first match is used')}
        </span>
      </div>

      {mode === 'json' && (
        <div className='space-y-3'>
          {!validation.success ? (
            <Alert variant='destructive'>
              <AlertDescription>{t(validation.message)}</AlertDescription>
            </Alert>
          ) : null}
          <JsonCodeEditor
            value={props.value}
            onChange={props.onChange}
            onBlur={props.onBlur}
            ariaLabel={t('Codex error response mappings JSON')}
            aria-invalid={!validation.success}
            heightClassName='h-96 min-h-96 max-h-96'
          />
        </div>
      )}
      {mode === 'visual' && rules && rules.length > 0 && (
        <div className='space-y-3'>
          {rules.map((rule, index) => {
            const issues = getCodexErrorResponseMappingIssues(rule)
            const showIssues = props.invalid
            const sourceStatusError = showIssues
              ? issues.sourceStatus
              : undefined
            const messagePatternsError = showIssues
              ? issues.messagePatterns
              : undefined
            const targetStatusError = showIssues
              ? issues.targetStatus
              : undefined
            const typeError = showIssues ? issues.type : undefined
            const codeError = showIssues ? issues.code : undefined
            const messageError = showIssues ? issues.message : undefined
            const ruleTitleId = `${fieldId}-rule-${index}-title`
            const patternsHelpId = `${fieldId}-rule-${index}-patterns-help`
            const patternsErrorId = `${fieldId}-rule-${index}-patterns-error`
            const sourceStatusErrorId = `${fieldId}-rule-${index}-source-status-error`
            const targetStatusErrorId = `${fieldId}-rule-${index}-target-status-error`
            const typeErrorId = `${fieldId}-rule-${index}-type-error`
            const codeErrorId = `${fieldId}-rule-${index}-code-error`
            const messageErrorId = `${fieldId}-rule-${index}-message-error`
            const patterns = rule.match.message_patterns
              .map((pattern) => pattern.trim())
              .filter(Boolean)
            const responsePreview = JSON.stringify(
              {
                error: {
                  type: rule.rewrite.type,
                  code: rule.rewrite.code,
                  message: rule.rewrite.message,
                },
              },
              null,
              2
            )

            return (
              <Card key={index} size='sm' aria-labelledby={ruleTitleId}>
                <CardHeader className='border-b'>
                  <div className='flex min-w-0 flex-wrap items-center gap-2'>
                    <CardTitle id={ruleTitleId}>
                      {t('Mapping rule {{number}}', { number: index + 1 })}
                    </CardTitle>
                    <Badge variant='outline'>{t('Any message matches')}</Badge>
                  </div>
                  <CardAction>
                    <Button
                      type='button'
                      variant='ghost'
                      size='icon-sm'
                      onClick={() =>
                        syncRules(
                          rules.filter((_, ruleIndex) => ruleIndex !== index)
                        )
                      }
                      aria-label={t('Delete mapping rule {{number}}', {
                        number: index + 1,
                      })}
                    >
                      <Trash2 aria-hidden='true' />
                    </Button>
                  </CardAction>
                </CardHeader>
                <CardContent className='grid min-w-0 gap-6 lg:grid-cols-2 lg:gap-0'>
                  <div className='min-w-0 space-y-4 lg:border-r lg:pr-4'>
                    <h4 className='text-sm font-medium'>
                      {t('Match conditions')}
                    </h4>
                    <div className='grid gap-4 sm:grid-cols-2'>
                      <div className='space-y-2'>
                        <Label htmlFor={`${fieldId}-rule-${index}-path`}>
                          {t('Endpoint path')}
                        </Label>
                        <Input
                          id={`${fieldId}-rule-${index}-path`}
                          value='/v1/responses'
                          readOnly
                        />
                      </div>
                      <div className='space-y-2'>
                        <Label
                          htmlFor={`${fieldId}-rule-${index}-source-status`}
                        >
                          {t('Upstream status code')}
                        </Label>
                        <Input
                          id={`${fieldId}-rule-${index}-source-status`}
                          type='number'
                          min={400}
                          max={599}
                          value={rule.match.status_code}
                          onBlur={props.onBlur}
                          onChange={(event) =>
                            updateRule(index, {
                              ...rule,
                              match: {
                                ...rule.match,
                                status_code: Number(event.target.value),
                              },
                            })
                          }
                          aria-invalid={Boolean(sourceStatusError)}
                          aria-describedby={
                            sourceStatusError ? sourceStatusErrorId : undefined
                          }
                        />
                        <FieldError id={sourceStatusErrorId}>
                          {sourceStatusError ? t(sourceStatusError) : undefined}
                        </FieldError>
                      </div>
                    </div>
                    <div className='space-y-2'>
                      <Label htmlFor={`${fieldId}-rule-${index}-patterns`}>
                        {t('Message patterns')}
                      </Label>
                      <Textarea
                        id={`${fieldId}-rule-${index}-patterns`}
                        value={rule.match.message_patterns.join('\n')}
                        rows={4}
                        onBlur={props.onBlur}
                        onChange={(event) =>
                          updateRule(index, {
                            ...rule,
                            match: {
                              ...rule.match,
                              message_patterns: event.target.value.split('\n'),
                            },
                          })
                        }
                        aria-invalid={Boolean(messagePatternsError)}
                        aria-describedby={
                          messagePatternsError
                            ? `${patternsHelpId} ${patternsErrorId}`
                            : patternsHelpId
                        }
                      />
                      <p
                        id={patternsHelpId}
                        className='text-muted-foreground text-xs'
                      >
                        {t(
                          'Enter one condition per line, use plain text for case-insensitive contains matching, or prefix a regular expression with re:'
                        )}
                      </p>
                      {patterns.length > 0 ? (
                        <div className='flex flex-wrap gap-1.5'>
                          {patterns.map((pattern, patternIndex) => (
                            <Badge
                              key={`${pattern}-${patternIndex}`}
                              variant='secondary'
                              className='max-w-full'
                            >
                              <span className='shrink-0'>
                                {pattern.startsWith('re:')
                                  ? t('Regex')
                                  : t('Contains')}
                              </span>
                              <span className='truncate font-mono'>
                                {pattern.startsWith('re:')
                                  ? pattern.slice(3)
                                  : pattern}
                              </span>
                            </Badge>
                          ))}
                        </div>
                      ) : null}
                      <FieldError id={patternsErrorId}>
                        {messagePatternsError
                          ? t(messagePatternsError)
                          : undefined}
                      </FieldError>
                      <p className='text-muted-foreground text-xs'>
                        {t(
                          'If any message pattern matches, the response below is returned'
                        )}
                      </p>
                    </div>
                  </div>

                  <div className='min-w-0 space-y-4 lg:pl-4'>
                    <h4 className='text-sm font-medium'>
                      {t('Response rewrite')}
                    </h4>
                    <div className='grid gap-4 sm:grid-cols-3'>
                      <div className='space-y-2'>
                        <Label
                          htmlFor={`${fieldId}-rule-${index}-target-status`}
                        >
                          {t('Return status code')}
                        </Label>
                        <Input
                          id={`${fieldId}-rule-${index}-target-status`}
                          type='number'
                          min={400}
                          max={599}
                          value={rule.rewrite.status_code}
                          onBlur={props.onBlur}
                          onChange={(event) =>
                            updateRule(index, {
                              ...rule,
                              rewrite: {
                                ...rule.rewrite,
                                status_code: Number(event.target.value),
                              },
                            })
                          }
                          aria-invalid={Boolean(targetStatusError)}
                          aria-describedby={
                            targetStatusError ? targetStatusErrorId : undefined
                          }
                        />
                        <FieldError id={targetStatusErrorId}>
                          {targetStatusError ? t(targetStatusError) : undefined}
                        </FieldError>
                      </div>
                      <div className='space-y-2'>
                        <Label htmlFor={`${fieldId}-rule-${index}-type`}>
                          {t('Error type')}
                        </Label>
                        <Input
                          id={`${fieldId}-rule-${index}-type`}
                          value={rule.rewrite.type}
                          onBlur={props.onBlur}
                          onChange={(event) =>
                            updateRule(index, {
                              ...rule,
                              rewrite: {
                                ...rule.rewrite,
                                type: event.target.value,
                              },
                            })
                          }
                          aria-invalid={Boolean(typeError)}
                          aria-describedby={typeError ? typeErrorId : undefined}
                        />
                        <FieldError id={typeErrorId}>
                          {typeError ? t(typeError) : undefined}
                        </FieldError>
                      </div>
                      <div className='space-y-2'>
                        <Label htmlFor={`${fieldId}-rule-${index}-code`}>
                          {t('Error code')}
                        </Label>
                        <Input
                          id={`${fieldId}-rule-${index}-code`}
                          value={rule.rewrite.code}
                          onBlur={props.onBlur}
                          onChange={(event) =>
                            updateRule(index, {
                              ...rule,
                              rewrite: {
                                ...rule.rewrite,
                                code: event.target.value,
                              },
                            })
                          }
                          aria-invalid={Boolean(codeError)}
                          aria-describedby={codeError ? codeErrorId : undefined}
                        />
                        <FieldError id={codeErrorId}>
                          {codeError ? t(codeError) : undefined}
                        </FieldError>
                      </div>
                    </div>
                    <div className='space-y-2'>
                      <Label htmlFor={`${fieldId}-rule-${index}-message`}>
                        {t('Error message')}
                      </Label>
                      <Input
                        id={`${fieldId}-rule-${index}-message`}
                        value={rule.rewrite.message}
                        onBlur={props.onBlur}
                        onChange={(event) =>
                          updateRule(index, {
                            ...rule,
                            rewrite: {
                              ...rule.rewrite,
                              message: event.target.value,
                            },
                          })
                        }
                        aria-invalid={Boolean(messageError)}
                        aria-describedby={
                          messageError ? messageErrorId : undefined
                        }
                      />
                      <FieldError id={messageErrorId}>
                        {messageError ? t(messageError) : undefined}
                      </FieldError>
                    </div>
                    <div className='bg-muted/40 overflow-hidden rounded-lg'>
                      <div className='flex flex-wrap items-center justify-between gap-2 border-b px-3 py-2'>
                        <span className='text-xs font-medium'>
                          {t('Response preview')}
                        </span>
                        <span className='text-muted-foreground text-xs'>
                          HTTP {rule.rewrite.status_code} · application/json
                        </span>
                      </div>
                      <pre className='max-h-56 overflow-auto p-3 font-mono text-xs'>
                        {responsePreview}
                      </pre>
                    </div>
                  </div>
                </CardContent>
              </Card>
            )
          })}
        </div>
      )}
      {mode === 'visual' && (!rules || rules.length === 0) && (
        <div className='text-muted-foreground flex min-h-28 items-center justify-center rounded-lg border border-dashed px-4 text-center text-sm'>
          {t(
            'No error response mappings are configured, so Codex clients receive the original upstream error'
          )}
        </div>
      )}
    </SettingsFormSection>
  )
}
