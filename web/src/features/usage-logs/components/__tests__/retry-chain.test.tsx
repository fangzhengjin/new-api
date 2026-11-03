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
import {
  flexRender,
  getCoreRowModel,
  useReactTable,
} from '@tanstack/react-table'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import i18next from 'i18next'
import { beforeAll, describe, expect, test, vi } from 'vitest'

import type { UsageLog } from '../../data/schema'
import { useCommonLogsColumns } from '../columns/common-logs-columns'
import { UsageLogsProvider } from '../usage-logs-provider'

vi.mock('@/lib/lobe-icon', () => ({
  getLobeIcon: () => null,
}))

Object.defineProperty(Element.prototype, 'getAnimations', {
  configurable: true,
  value: () => [],
})

function ChannelCell(props: { log: UsageLog }) {
  const columns = useCommonLogsColumns(true, false)
  const table = useReactTable({
    columns,
    data: [props.log],
    getCoreRowModel: getCoreRowModel(),
  })
  const cell = table
    .getRowModel()
    .rows[0]?.getVisibleCells()
    .find((candidate) => candidate.column.id === 'channel')

  return cell ? flexRender(cell.column.columnDef.cell, cell.getContext()) : null
}

function createLog(): UsageLog {
  return {
    id: 1,
    user_id: 1,
    created_at: 1000,
    type: 2,
    content: '',
    username: 'test',
    display_name: '',
    token_name: '',
    model_name: 'gpt-test',
    quota: 1,
    prompt_tokens: 1,
    completion_tokens: 0,
    use_time: 1,
    is_stream: false,
    channel: 2,
    channel_name: 'Multi-key channel',
    token_id: 1,
    group: 'default',
    ip: '',
    other: JSON.stringify({
      admin_info: {
        is_multi_key: true,
        multi_key_index: 7,
        use_channel: ['2', '2'],
        retry_targets: [
          {
            channel_id: 2,
            multi_key_index: 3,
            status_code: 429,
            error: 'rate limited',
          },
          { channel_id: 2, multi_key_index: 7 },
        ],
      },
    }),
    request_id: 'request-1',
    upstream_request_id: '',
  }
}

describe('usage log retry chain', () => {
  beforeAll(() => {
    i18next.addResourceBundle('en', 'translation', {
      Key: 'Key',
      'Retry Chain': 'Retry Chain',
    })
  })

  test('shows the final key badge and retry details from one log row', async () => {
    const user = userEvent.setup()
    render(
      <UsageLogsProvider>
        <ChannelCell log={createLog()} />
      </UsageLogsProvider>
    )

    expect(screen.getByLabelText('Key 7')).toBeVisible()
    await user.click(screen.getByRole('button', { name: 'Retry Chain' }))

    expect(
      await screen.findByText('#2 / Key 3 · 429: rate limited → #2 / Key 7')
    ).toBeVisible()
  })
})
