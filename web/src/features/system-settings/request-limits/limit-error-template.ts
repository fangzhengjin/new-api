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

export const REQUEST_LIMIT_ERROR_TEMPLATE_MAX_LENGTH = 2000

export const createLimitErrorTemplateSchema = (t: (key: string) => string) =>
  z
    .string()
    .refine(
      (value) => [...value].length <= REQUEST_LIMIT_ERROR_TEMPLATE_MAX_LENGTH,
      t('Enter no more than 2000 characters')
    )

/** Formats the language-neutral duration used by request-limit error messages */
export function formatRequestLimitDuration(seconds: number) {
  const value =
    Number.isFinite(seconds) && seconds > 0 ? Math.floor(seconds) : 0
  if (value < 60) return `${value}s`
  if (value < 60 * 60) {
    const minutes = Math.floor(value / 60)
    const remainingSeconds = value % 60
    return remainingSeconds === 0
      ? `${minutes}m`
      : `${minutes}m ${remainingSeconds}s`
  }
  const hours = Math.floor(value / (60 * 60))
  const minutes = Math.floor((value % (60 * 60)) / 60)
  return minutes === 0 ? `${hours}h` : `${hours}h ${minutes}m`
}
