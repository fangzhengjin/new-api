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
import assert from 'node:assert/strict'

import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { describe, test, vi } from 'vitest'

import type { UsageLog } from '../../data/schema'

vi.mock('@/lib/lobe-icon', () => ({
  getLobeIcon: () => null,
}))

Object.defineProperty(Element.prototype, 'getAnimations', {
  configurable: true,
  value: () => [],
})

const { act } = await import('react')
const { createRoot } = await import('react-dom/client')
const { createInstance } = await import('i18next')
const { I18nextProvider, initReactI18next } = await import('react-i18next')
const { flexRender, getCoreRowModel, useReactTable } =
  await import('@tanstack/react-table')
const { useCommonLogsColumns } = await import('../columns/common-logs-columns')
const { UsageLogsMobileList } = await import('../usage-logs-mobile-card')
const { UsageLogsProvider } = await import('../usage-logs-provider')

const i18n = createInstance()
await i18n.use(initReactI18next).init({ lng: 'en' })

const reactTestGlobals = globalThis as typeof globalThis & {
  IS_REACT_ACT_ENVIRONMENT?: boolean
}
reactTestGlobals.IS_REACT_ACT_ENVIRONMENT = true

function createLog(displayName: string): UsageLog & { display_name: string } {
  return {
    id: 1,
    user_id: 1,
    created_at: 1000,
    type: 2,
    content: '',
    username: 'historical-alice',
    display_name: displayName,
    token_name: '',
    model_name: '',
    quota: 0,
    prompt_tokens: 0,
    completion_tokens: 0,
    use_time: 0,
    is_stream: false,
    channel: 0,
    channel_name: '',
    token_id: 0,
    group: '',
    ip: '',
    other: '',
    request_id: '',
    upstream_request_id: '',
  }
}

function CommonLogHarness(props: { log: UsageLog; mobile?: boolean }) {
  const columns = useCommonLogsColumns(true, false)
  const table = useReactTable({
    columns,
    data: [props.log],
    getCoreRowModel: getCoreRowModel(),
  })

  if (props.mobile) {
    return <UsageLogsMobileList table={table} logCategory='common' />
  }

  const cell = table
    .getRowModel()
    .rows[0]?.getVisibleCells()
    .find((candidate) => candidate.column.id === 'user')

  return cell ? flexRender(cell.column.columnDef.cell, cell.getContext()) : null
}

async function renderUserCell(log: UsageLog, mobile = false) {
  const container = document.createElement('div')
  document.body.append(container)
  const root = createRoot(container)
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  const freshAt = Date.now() + 60_000
  queryClient.setQueryData(['status'], {}, { updatedAt: freshAt })
  queryClient.setQueryData(
    ['pricing'],
    { data: [], vendors: [] },
    { updatedAt: freshAt }
  )

  await act(async () => {
    root.render(
      <QueryClientProvider client={queryClient}>
        <I18nextProvider i18n={i18n}>
          <UsageLogsProvider>
            <CommonLogHarness log={log} mobile={mobile} />
          </UsageLogsProvider>
        </I18nextProvider>
      </QueryClientProvider>
    )
  })

  return { container, root, queryClient }
}

async function unmountUserCell(
  rendered: Awaited<ReturnType<typeof renderUserCell>>
) {
  await act(async () => rendered.root.unmount())
  rendered.queryClient.clear()
  rendered.container.remove()
}

function findExactText(
  container: HTMLElement,
  text: string
): HTMLElement | undefined {
  return [...container.querySelectorAll<HTMLElement>('span')].find(
    (element) => element.textContent === text
  )
}

describe('usage log user display name', () => {
  test('shows a distinct display name below the historical username', async () => {
    const rendered = await renderUserCell(createLog('Alice Chen'))
    const username = findExactText(rendered.container, 'historical-alice')
    const displayName = findExactText(rendered.container, 'Alice Chen')

    assert.ok(username)
    assert.ok(displayName)
    assert.equal(username.parentElement, displayName.parentElement)

    await unmountUserCell(rendered)
  })

  test('does not repeat a display name equal to the username', async () => {
    const rendered = await renderUserCell(createLog('historical-alice'))
    const matches = [...rendered.container.querySelectorAll('span')].filter(
      (element) => element.textContent === 'historical-alice'
    )

    assert.equal(matches.length, 1)

    await unmountUserCell(rendered)
  })

  test('shows the same two-level identity in the mobile common-log card', async () => {
    const rendered = await renderUserCell(createLog('Alice Chen'), true)
    const username = findExactText(rendered.container, 'historical-alice')
    const displayName = findExactText(rendered.container, 'Alice Chen')

    assert.ok(username)
    assert.ok(displayName)
    assert.equal(username.parentElement, displayName.parentElement)

    await unmountUserCell(rendered)
  })
})
