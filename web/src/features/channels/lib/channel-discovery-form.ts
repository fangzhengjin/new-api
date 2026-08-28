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

/** Validates the pasted connection source before starting discovery. */
export const discoveryTextSchema = z
  .string()
  .trim()
  .min(1, 'Enter an upstream URL and at least one API key')

/** Validates every editable field used to build a discovery preview draft. */
export const discoveryFormSchema = z.object({
  text: discoveryTextSchema,
  operation: z.enum(['create', 'update']),
  channelId: z.number().int().nonnegative(),
  baseUrl: z.string().trim().url('Enter a valid Base URL'),
  name: z.string().trim(),
  tag: z.string().trim(),
  priority: z.number().int(),
  keyMode: z.enum(['append', 'replace']),
  syncConfiguration: z.boolean(),
  enabled: z.boolean(),
  reenable: z.boolean(),
})

/** Contains validated smart-recognition form values inferred from the schema. */
export type DiscoveryFormValues = z.infer<typeof discoveryFormSchema>

/** Shares page layout, selection state, and touch-target classes across setup UI. */
export const channelSetupLayoutClasses = {
  content: 'mx-auto flex w-full max-w-7xl flex-col gap-4',
  workspace:
    'grid items-start gap-4 xl:grid-cols-[minmax(18rem,22rem)_minmax(0,1fr)]',
  footer:
    'flex w-full flex-col gap-2 sm:flex-row sm:items-center sm:justify-end',
  discoveryResult:
    'data-[selected=true]:border-primary/55 data-[selected=true]:bg-muted',
  mobileAction: 'max-sm:min-h-11',
  mobileControls:
    'max-sm:[&_[data-slot=button]]:min-h-11 max-sm:[&_[data-slot=input]]:h-11 max-sm:[&_[data-slot=native-select]]:h-11 max-sm:[&_[data-slot=tabs-list]]:h-11 max-sm:[&_[data-orientation=horizontal]>[data-slot=field-label]]:min-h-11 max-sm:[&_[data-orientation=horizontal]>[data-slot=field-label]]:items-center',
} as const
