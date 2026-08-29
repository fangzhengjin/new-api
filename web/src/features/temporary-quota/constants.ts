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
import type { StatusVariant } from '@/components/status-badge'

import type { TemporaryQuotaRequestStatus } from './types'

export const TEMPORARY_QUOTA_REQUEST_STATUSES: Record<
  TemporaryQuotaRequestStatus,
  { labelKey: string; variant: StatusVariant }
> = {
  pending: { labelKey: 'Pending review', variant: 'warning' },
  executed: { labelKey: 'Granted', variant: 'success' },
  rejected: { labelKey: 'Rejected', variant: 'danger' },
}

export const TEMPORARY_QUOTA_REQUEST_STATUS_OPTIONS = Object.entries(
  TEMPORARY_QUOTA_REQUEST_STATUSES
).map(([value, config]) => ({ value, label: config.labelKey }))
