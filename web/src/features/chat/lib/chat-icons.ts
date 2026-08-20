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
import { iconNames, type IconName } from 'lucide-react/dynamic'

const chatPresetIconNames = new Set<string>(iconNames)

/** Converts common Lucide component-name formats to the dynamic icon slug. */
export function normalizeChatPresetIconName(value: string): string {
  return value
    .trim()
    .replaceAll(/([A-Z]+)([A-Z][a-z])/g, '$1-$2')
    .replaceAll(/([a-z0-9])([A-Z])/g, '$1-$2')
    .replaceAll(/[\s_]+/g, '-')
    .toLowerCase()
}

/** Returns the matching Lucide icon slug, or null when the name is unknown. */
export function resolveChatPresetIconName(value?: string): IconName | null {
  if (!value) return null
  const normalized = normalizeChatPresetIconName(value)
  return chatPresetIconNames.has(normalized) ? (normalized as IconName) : null
}

/** Reports whether a configured chat-preset icon exists in Lucide. */
export function isChatPresetIconName(value: string): boolean {
  return resolveChatPresetIconName(value) !== null
}
