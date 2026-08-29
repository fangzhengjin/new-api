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
import { act, render, screen, waitFor } from '@testing-library/react'
import type { ComponentType, ReactNode } from 'react'
import { beforeEach, describe, expect, test, vi } from 'vitest'

import { Route } from '@/routes/_authenticated/chat/$chatId'

const mocks = vi.hoisted(() => ({
  navigate: vi.fn(),
  prepareLaunch: vi.fn(),
  launchWithTokenId: vi.fn(),
  dismiss: undefined as (() => void) | undefined,
}))

vi.mock('@tanstack/react-router', () => ({
  Link: (props: { children?: ReactNode; to?: string }) => (
    <a href={props.to}>{props.children}</a>
  ),
  createFileRoute: () => (options: Record<string, unknown>) => ({
    options,
    useNavigate: () => mocks.navigate,
    useParams: () => ({ chatId: '0' }),
    useSearch: () => ({}),
  }),
  redirect: vi.fn(),
}))

vi.mock('@/context/theme-provider', () => ({
  useTheme: () => ({ resolvedTheme: 'light' }),
}))

vi.mock('@/features/chat/hooks/use-chat-presets', () => ({
  useChatPresets: () => ({
    chatPresets: [
      {
        id: '0',
        name: 'Canvas',
        url: 'https://canvas.example/?text={textModels}',
        type: 'web',
        openMode: 'embedded',
      },
    ],
    serverAddress: 'https://api.example.test',
  }),
}))

vi.mock('@/features/chat/components/chat-preset-launcher', () => ({
  useChatPresetLauncher: (...args: unknown[]) => {
    mocks.dismiss = args[3] as (() => void) | undefined
    return {
      loadingPresetId: null,
      launchWithTokenId: mocks.launchWithTokenId,
      prepareLaunch: mocks.prepareLaunch,
      dialog: null,
    }
  },
}))

const RouteComponent = Route.options.component as ComponentType

describe('protected embedded chat route launch', () => {
  beforeEach(() => {
    mocks.navigate.mockReset()
    mocks.prepareLaunch.mockReset()
    mocks.launchWithTokenId.mockReset()
    mocks.dismiss = undefined
  })

  test('prepares Token selection behind a single accessible loading state', async () => {
    render(<RouteComponent />)

    await waitFor(() => expect(mocks.prepareLaunch).toHaveBeenCalledTimes(1))
    expect(
      screen.getByRole('status', { name: 'Preparing your chat link…' })
    ).toBeInTheDocument()
    expect(
      screen.queryByRole('heading', { name: 'Choose a Token' })
    ).not.toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: 'Choose a Token' })
    ).not.toBeInTheDocument()
  })

  test('leaves the route when direct Token selection is dismissed', () => {
    render(<RouteComponent />)

    expect(mocks.dismiss).toBeTypeOf('function')
    act(() => mocks.dismiss?.())
    expect(mocks.navigate).toHaveBeenCalledWith({
      to: '/dashboard',
      replace: true,
    })
  })
})
