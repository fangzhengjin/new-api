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
import { z } from 'zod'

export function createTemporaryQuotaFormSchema(options: {
  amountPattern: RegExp
  amountError: string
  amountIsValid: (value: string) => boolean
  projectError: string
  reasonRequiredError: string
  reasonLengthError: string
}) {
  return z.object({
    amount: z
      .string()
      .regex(options.amountPattern, options.amountError)
      .refine(options.amountIsValid, options.amountError),
    project: z.string().min(1, options.projectError),
    reason: z
      .string()
      .trim()
      .min(1, options.reasonRequiredError)
      .refine((value) => [...value].length <= 500, options.reasonLengthError),
  })
}

export type TemporaryQuotaFormValues = z.infer<
  ReturnType<typeof createTemporaryQuotaFormSchema>
>
