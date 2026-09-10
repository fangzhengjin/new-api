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
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'

import type { UsageLog } from '../../../data/schema'
import { DetailsDialog } from '../details-dialog'

Object.defineProperty(Element.prototype, 'getAnimations', {
  configurable: true,
  value: () => [],
})

const queryClient = new QueryClient({
  defaultOptions: { queries: { retry: false } },
})

beforeEach(() => {
  const freshAt = Date.now() + 60_000
  queryClient.setQueryData(['status'], {}, { updatedAt: freshAt })
  queryClient.setQueryData(
    ['pricing'],
    { data: [], vendors: [] },
    { updatedAt: freshAt }
  )
})

afterEach(() => {
  queryClient.clear()
  vi.restoreAllMocks()
})

function renderDetails(log: UsageLog, isAdmin: boolean) {
  return render(
    <DetailsDialog
      log={log}
      isAdmin={isAdmin}
      isRoot={false}
      open
      onOpenChange={() => undefined}
    />,
    {
      wrapper: ({ children }) => (
        <QueryClientProvider client={queryClient}>
          {children}
        </QueryClientProvider>
      ),
    }
  )
}

const codexTurnMetadata = JSON.stringify({
  installation_id: 'installation-id',
  session_id: 'session-id',
  thread_id: 'thread-id',
  turn_id: 'turn-id',
  request_kind: 'turn',
})

const outgoingUserAgent =
  'codex-tui/0.147.0 (Mac OS 26.5.2; arm64) ghostty/1.3.1 (codex-tui; 0.147.0)'
const wideUserAgent =
  'Codex Desktop/0.148.0-alpha.15 (Windows 10.0.26200; x86_64) unknown (Codex Desktop; 26.814.41407)'

function mockHeaderHeights(overflownText: string[]) {
  vi.spyOn(HTMLElement.prototype, 'clientHeight', 'get').mockImplementation(
    function (this: HTMLElement) {
      return this.tagName === 'CODE' ? 40 : 0
    }
  )
  vi.spyOn(HTMLElement.prototype, 'scrollHeight', 'get').mockImplementation(
    function (this: HTMLElement) {
      if (this.tagName !== 'CODE') return 0
      return overflownText.some((text) => this.textContent?.includes(text))
        ? 60
        : 20
    }
  )
}

const log: UsageLog = {
  id: 1,
  user_id: 1,
  created_at: 1000,
  type: 5,
  content: 'error',
  username: 'test',
  display_name: '',
  token_name: 'token',
  model_name: 'gpt-5',
  quota: 0,
  prompt_tokens: 0,
  completion_tokens: 0,
  use_time: 0,
  is_stream: false,
  channel: 1,
  channel_name: 'channel',
  token_id: 1,
  group: 'default',
  ip: '',
  request_id: 'request-id',
  upstream_request_id: '',
  other: JSON.stringify({
    request_path: '/v1/responses',
    request_conversion: ['OpenAI', 'OpenAI Responses'],
    reasoning_effort: { original: 'high', final: 'max' },
    is_model_mapped: true,
    upstream_model_name: 'free/gpt-5',
    admin_info: {
      request_headers: {
        incoming: {
          'User-Agent': 'OpenAI/Python 2.24.0',
          Originator: 'custom-client',
        },
        outgoing: {
          'User-Agent': outgoingUserAgent,
          Originator: 'codex-tui',
          'X-Codex-Turn-Metadata': codexTurnMetadata,
          'X-OpenAI-Subagent': 'guardian',
        },
        omitted: {
          outgoing: [{ name: 'X-Large-Debug', byte_length: 20000 }],
        },
      },
    },
  }),
}

describe('Request header audit details', () => {
  test('shows the narrow User-Agent transition and expands other headers only to administrators', async () => {
    mockHeaderHeights([outgoingUserAgent, codexTurnMetadata])
    const user = userEvent.setup()
    const { rerender } = renderDetails(log, true)

    expect(await screen.findByText('OpenAI/Python 2.24.0')).toBeInTheDocument()
    expect(screen.getByText(outgoingUserAgent)).toBeInTheDocument()
    expect(screen.queryByText('custom-client')).not.toBeInTheDocument()

    const userAgentDetails = screen.getByRole('button', {
      name: 'User-Agent: Details',
    })
    expect(userAgentDetails).toHaveAttribute('aria-expanded', 'false')
    fireEvent.click(userAgentDetails)
    expect(userAgentDetails).toHaveAttribute('aria-expanded', 'true')

    const requestHeadersToggle = screen.getByRole('button', { name: 'Expand' })
    expect(requestHeadersToggle).toHaveAttribute('aria-expanded', 'false')
    fireEvent.click(requestHeadersToggle)
    expect(screen.getByRole('button', { name: 'Collapse' })).toHaveAttribute(
      'aria-expanded',
      'true'
    )
    expect(await screen.findByText('custom-client')).toBeInTheDocument()
    expect(screen.getByText('codex-tui')).toBeInTheDocument()
    expect(screen.getByText('guardian')).toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: 'Originator: Details' })
    ).not.toBeInTheDocument()

    const metadataDetails = screen.getByRole('button', {
      name: 'X-Codex-Turn-Metadata: Details',
    })
    const metadataGroup = screen.getByRole('group', {
      name: /^X-Codex-Turn-Metadata:/,
    })
    metadataDetails.focus()
    await user.keyboard('{Enter}')
    expect(metadataDetails).toHaveAttribute('aria-expanded', 'true')
    expect(metadataDetails).toHaveFocus()
    expect(within(metadataGroup).getAllByText(codexTurnMetadata)).toHaveLength(
      1
    )
    expect(metadataGroup.querySelector('pre')).not.toBeInTheDocument()
    expect(screen.getByText('X-Large-Debug')).toBeInTheDocument()
    expect(screen.getByText('20000 bytes')).toBeInTheDocument()
    expect(
      screen.queryByRole('link', { name: 'View audit rules' })
    ).not.toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'Collapse' }))
    expect(screen.getByRole('button', { name: 'Expand' })).toHaveAttribute(
      'aria-expanded',
      'false'
    )

    const reasoning = screen.getByText('Reasoning Effort')
    const conversion = screen.getByText('Request Conversion')
    const mapping = screen.getByText('Model Mapping')
    const headers = screen.getByText('Request Headers')
    expect(
      reasoning.compareDocumentPosition(conversion) &
        Node.DOCUMENT_POSITION_FOLLOWING
    ).toBeTruthy()
    expect(
      conversion.compareDocumentPosition(mapping) &
        Node.DOCUMENT_POSITION_FOLLOWING
    ).toBeTruthy()
    expect(
      mapping.compareDocumentPosition(headers) &
        Node.DOCUMENT_POSITION_FOLLOWING
    ).toBeTruthy()

    rerender(
      <DetailsDialog
        log={log}
        isAdmin={false}
        isRoot={false}
        open
        onOpenChange={() => undefined}
      />
    )

    expect(screen.queryByText('OpenAI/Python 2.24.0')).not.toBeInTheDocument()
    expect(screen.queryByText('Request Headers')).not.toBeInTheDocument()
  })

  test('hides User-Agent details when the rendered text fits within two lines', async () => {
    mockHeaderHeights([])
    const fittingLog = {
      ...log,
      other: JSON.stringify({
        admin_info: {
          request_headers: { incoming: { 'User-Agent': wideUserAgent } },
        },
      }),
    }

    renderDetails(fittingLog, true)

    expect(await screen.findByText(wideUserAgent)).toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: 'User-Agent: Details' })
    ).not.toBeInTheDocument()
  })

  test('keeps details operable for overflowing header text', async () => {
    mockHeaderHeights([wideUserAgent])
    const user = userEvent.setup()
    const overflowingLog = {
      ...log,
      other: JSON.stringify({
        admin_info: {
          request_headers: { incoming: { 'User-Agent': wideUserAgent } },
        },
      }),
    }

    renderDetails(overflowingLog, true)

    const details = await screen.findByRole('button', {
      name: 'User-Agent: Details',
    })
    details.focus()
    await user.keyboard('{Enter}')
    expect(details).toHaveAttribute('aria-expanded', 'true')
    expect(details).toHaveFocus()
  })
})
