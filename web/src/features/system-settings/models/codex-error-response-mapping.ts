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
import * as z from 'zod'

const httpStatusSchema = z
  .number()
  .int('Enter an HTTP error status code from 400 to 599')
  .min(400, 'Enter an HTTP error status code from 400 to 599')
  .max(599, 'Enter an HTTP error status code from 400 to 599')

const messagePatternSchema = z
  .string()
  .trim()
  .min(1, 'Enter at least one message pattern')
  .max(1024, 'Message pattern is too long')

const messagePatternsSchema = z
  .array(z.string())
  .transform((patterns) =>
    patterns.map((pattern) => pattern.trim()).filter(Boolean)
  )
  .pipe(
    z
      .array(messagePatternSchema)
      .min(1, 'Enter at least one message pattern')
      .max(100, 'No more than 100 message patterns are allowed')
  )

const requiredRewriteValue = z.string().trim().min(1, 'This field is required')

const codexErrorResponseMatchDraftShape = {
  status_code: z.number(),
  message_patterns: z.array(z.string()),
}

const codexErrorResponseRewriteDraftShape = {
  status_code: z.number(),
  type: z.string(),
  code: z.string(),
  message: z.string(),
}

const codexErrorResponseMappingDraftsSchema = z.array(
  z
    .object({
      match: z.object(codexErrorResponseMatchDraftShape).passthrough(),
      rewrite: z.object(codexErrorResponseRewriteDraftShape).passthrough(),
    })
    .passthrough()
)

export const codexErrorResponseMappingSchema = z.object({
  match: z.object({
    ...codexErrorResponseMatchDraftShape,
    status_code: httpStatusSchema,
    message_patterns: messagePatternsSchema,
  }),
  rewrite: z.object({
    ...codexErrorResponseRewriteDraftShape,
    status_code: httpStatusSchema,
    type: requiredRewriteValue.max(128, 'This field is too long'),
    code: requiredRewriteValue.max(128, 'This field is too long'),
    message: requiredRewriteValue.max(4096, 'This field is too long'),
  }),
})

export const codexErrorResponseMappingsSchema = z
  .array(codexErrorResponseMappingSchema)
  .max(100, 'No more than 100 mapping rules are allowed')

export type CodexErrorResponseMapping = z.infer<
  typeof codexErrorResponseMappingSchema
>

export type CodexErrorResponseMappingIssues = Partial<
  Record<
    | 'sourceStatus'
    | 'messagePatterns'
    | 'targetStatus'
    | 'type'
    | 'code'
    | 'message',
    string
  >
>

type ParseResult =
  | { success: true; data: CodexErrorResponseMapping[] }
  | { success: false; message: string }

export const retryableRateLimitTemplate: CodexErrorResponseMapping = {
  match: {
    status_code: 429,
    message_patterns: [
      'Request rate increased too quickly.',
      'Requests rate limit exceeded',
      're:(request|token).*rate limit exceeded',
    ],
  },
  rewrite: {
    status_code: 503,
    type: 'server_error',
    code: 'server_is_overloaded',
    message: 'Temporary upstream rate limit',
  },
}

export function createEmptyCodexErrorResponseMapping(): CodexErrorResponseMapping {
  return {
    match: { status_code: 429, message_patterns: [''] },
    rewrite: {
      status_code: 503,
      type: 'server_error',
      code: 'server_is_overloaded',
      message: 'Temporary upstream rate limit',
    },
  }
}

export function parseCodexErrorResponseMappingDrafts(
  value: string
): CodexErrorResponseMapping[] | null {
  try {
    const result = codexErrorResponseMappingDraftsSchema.safeParse(
      JSON.parse(value)
    )
    return result.success ? result.data : null
  } catch {
    return null
  }
}

export function validateCodexErrorResponseMappings(value: string): ParseResult {
  let parsed: unknown
  try {
    parsed = JSON.parse(value)
  } catch {
    return { success: false, message: 'Enter a valid JSON array' }
  }
  const result = codexErrorResponseMappingsSchema.safeParse(parsed)
  if (!result.success) {
    return {
      success: false,
      message: result.error.issues[0]?.message ?? 'Invalid mapping rule',
    }
  }
  return { success: true, data: result.data }
}

export const codexErrorResponseMappingsFieldSchema = z
  .string()
  .superRefine((value, context) => {
    const result = validateCodexErrorResponseMappings(value)
    if (!result.success) {
      context.addIssue({ code: 'custom', message: result.message })
    }
  })

export function normalizeCodexErrorResponseMappings(value: string): string {
  const result = validateCodexErrorResponseMappings(value)
  return result.success ? JSON.stringify(result.data) : value.trim()
}

export function formatCodexErrorResponseMappings(value: string): string {
  const drafts = parseCodexErrorResponseMappingDrafts(value)
  return JSON.stringify(drafts ?? [], null, 2)
}

export function getCodexErrorResponseMappingIssues(
  mapping: CodexErrorResponseMapping
): CodexErrorResponseMappingIssues {
  const result = codexErrorResponseMappingSchema.safeParse(mapping)
  if (result.success) return {}

  const issues: CodexErrorResponseMappingIssues = {}
  for (const issue of result.error.issues) {
    const path = issue.path.join('.')
    if (path === 'match.status_code' && !issues.sourceStatus) {
      issues.sourceStatus = issue.message
    } else if (
      path.startsWith('match.message_patterns') &&
      !issues.messagePatterns
    ) {
      issues.messagePatterns = issue.message
    } else if (path === 'rewrite.status_code' && !issues.targetStatus) {
      issues.targetStatus = issue.message
    } else if (path === 'rewrite.type' && !issues.type) {
      issues.type = issue.message
    } else if (path === 'rewrite.code' && !issues.code) {
      issues.code = issue.message
    } else if (path === 'rewrite.message' && !issues.message) {
      issues.message = issue.message
    }
  }
  return issues
}
