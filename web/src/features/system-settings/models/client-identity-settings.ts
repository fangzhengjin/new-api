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
import i18n from 'i18next'
import * as z from 'zod'

export const modelPatternListSchema = z.string().superRefine((value, ctx) => {
  const patterns = value
    .split('\n')
    .map((pattern) => pattern.trim())
    .filter(Boolean)
  if (patterns.length === 0 || patterns.length > 64) {
    ctx.addIssue({
      code: z.ZodIssueCode.custom,
      message: i18n.t('Enter between 1 and 64 model patterns'),
    })
  } else if (patterns.some((pattern) => pattern.length > 256)) {
    ctx.addIssue({
      code: z.ZodIssueCode.custom,
      message: i18n.t('Each model pattern must be 256 characters or fewer'),
    })
  }
})

export function formatModelPatterns(value: string) {
  try {
    const patterns = JSON.parse(value)
    return Array.isArray(patterns) ? patterns.join('\n') : value
  } catch {
    return value
  }
}

export function serializeModelPatterns(value: string) {
  return JSON.stringify(
    value
      .split('\n')
      .map((pattern) => pattern.trim())
      .filter(Boolean)
  )
}
