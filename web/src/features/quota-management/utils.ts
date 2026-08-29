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
import { formatQuotaWithCurrency, getCurrencyDisplay } from '@/lib/currency'
import { parseQuotaFromDollars, quotaUnitsToDollars } from '@/lib/format'

const MAX_INT64 = 9_223_372_036_854_775_807n

export function formatQuota(value: string, signed = false): string {
  const { meta } = getCurrencyDisplay()
  if (meta.kind === 'tokens') {
    const quota = BigInt(value)
    let prefix = ''
    if (quota < 0n) prefix = '-'
    else if (signed && quota > 0n) prefix = '+'
    const absolute = quota < 0n ? -quota : quota
    if (absolute < 1000n) return `${prefix}${absolute}`

    const tenths = (absolute + 50n) / 100n
    const whole = tenths / 10n
    const fraction = tenths % 10n
    return `${prefix}${whole}${fraction === 0n ? '' : `.${fraction}`}k`
  }
  const quota = Number(value)
  const formatted = formatQuotaWithCurrency(quota, {
    digitsLarge: 2,
    digitsSmall: 4,
    abbreviate: true,
  })
  return signed && quota > 0 ? `+${formatted}` : formatted
}

export function quotaToDisplayAmount(value: string): string {
  const { meta } = getCurrencyDisplay()
  if (meta.kind === 'tokens') return BigInt(value).toString()

  const amount = quotaUnitsToDollars(Number(value))
  return amount.toFixed(6).replace(/\.?0+$/, '')
}

export function quotaFromDisplayAmount(
  value: string,
  allowZero = false
): string | null {
  const { meta } = getCurrencyDisplay()
  if (meta.kind === 'tokens') {
    if (!/^\d+$/.test(value)) return null
    const quota = BigInt(value)
    return (allowZero ? quota >= 0n : quota > 0n) && quota <= MAX_INT64
      ? quota.toString()
      : null
  }

  const amount = Number(value)
  if (!Number.isFinite(amount) || (allowZero ? amount < 0 : amount <= 0)) {
    return null
  }
  const quota = parseQuotaFromDollars(amount)
  return Number.isSafeInteger(quota) && (allowZero ? quota >= 0 : quota > 0)
    ? String(quota)
    : null
}

export function formatDateTime(timestamp?: number | null): string {
  if (!timestamp) return '—'
  return new Intl.DateTimeFormat(undefined, {
    timeZone: 'Asia/Shanghai',
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    hour12: false,
  }).format(timestamp * 1000)
}

export function toLocalInput(timestamp: number): string {
  const parts = new Intl.DateTimeFormat('sv-SE', {
    timeZone: 'Asia/Shanghai',
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    hour12: false,
  }).formatToParts(timestamp * 1000)
  const part = (type: Intl.DateTimeFormatPartTypes) =>
    parts.find((item) => item.type === type)?.value ?? ''
  return `${part('year')}-${part('month')}-${part('day')}T${part('hour')}:${part('minute')}`
}

export function fromShanghaiInput(value: string): number {
  return Math.floor(new Date(`${value}:00+08:00`).getTime() / 1000)
}

export function toShanghaiDateInput(timestamp: number): string {
  return toLocalInput(timestamp).slice(0, 10)
}

export function fromShanghaiAdjustmentDate(
  value: string,
  cycleEnd: number
): number {
  if (value === toShanghaiDateInput(cycleEnd)) {
    return cycleEnd
  }
  return Math.floor(new Date(`${value}T09:00:00+08:00`).getTime() / 1000)
}

export const queryKeys = {
  overview: ['quota-management', 'overview'] as const,
  activities: ['quota-management', 'activities'] as const,
  cycles: ['quota-management', 'cycles'] as const,
  options: ['quota-management', 'options'] as const,
  cycle: (id: number) => ['quota-management', 'cycle', id] as const,
  plan: (id: number) => ['quota-management', 'plan', id] as const,
  temporaryQuotaRequests: [
    'quota-management',
    'temporary-quota-requests',
  ] as const,
}
