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
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { describe, expect, test, vi } from 'vitest'

import { CodexSettingsCard } from '../codex-settings-card'

const { mutateAsync } = vi.hoisted(() => ({
  mutateAsync: vi.fn().mockResolvedValue(undefined),
}))

vi.mock('../../hooks/use-update-option', () => ({
  useUpdateOption: () => ({ mutateAsync, isPending: false }),
}))

describe('Codex settings card', () => {
  test('saves the global request-header fallback switch', async () => {
    const { container } = render(
      <CodexSettingsCard
        defaultValues={{
          codex: { request_header_fallback_enabled: false },
        }}
      />
    )

    fireEvent.click(
      screen.getByRole('switch', { name: 'Codex Request Header Fallback' })
    )
    const form = container.querySelector('form')
    expect(form).not.toBeNull()
    fireEvent.submit(form as HTMLFormElement)

    await waitFor(() => {
      expect(mutateAsync).toHaveBeenCalledWith({
        key: 'codex.request_header_fallback_enabled',
        value: true,
      })
    })
  })
})
