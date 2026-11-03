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
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, test, vi } from 'vitest'

import { ChatDialog } from '../chat-dialog'

vi.mock('lucide-react/dynamic', () => ({
  iconNames: ['message-square', 'palette'],
  DynamicIcon: ({ name }: { name: string }) => (
    <svg data-testid={`lucide-${name}`} />
  ),
}))

describe('chat preset dialog', () => {
  test('previews and saves a valid top-level menu icon', async () => {
    const onSave = vi.fn()
    const user = userEvent.setup()
    render(
      <ChatDialog open onOpenChange={vi.fn()} onSave={onSave} editData={null} />
    )

    const iconInput = screen.getByRole('textbox', { name: 'Menu icon' })
    expect(iconInput).toBe(screen.getAllByRole('textbox')[0])
    await user.type(iconInput, 'Palette')
    expect(await screen.findByTestId('lucide-palette')).toBeInTheDocument()
    await user.type(
      screen.getByRole('textbox', { name: 'Chat Client Name' }),
      'Canvas'
    )
    await user.type(
      screen.getByRole('textbox', { name: 'URL' }),
      'https://canvas.example'
    )
    await user.click(screen.getByRole('button', { name: 'Add' }))

    await waitFor(() =>
      expect(onSave).toHaveBeenCalledWith({
        icon: 'Palette',
        name: 'Canvas',
        url: 'https://canvas.example',
        openMode: 'embedded',
      })
    )
  })

  test('rejects an unknown menu icon name', async () => {
    const onSave = vi.fn()
    const user = userEvent.setup()
    render(
      <ChatDialog open onOpenChange={vi.fn()} onSave={onSave} editData={null} />
    )

    await user.type(
      screen.getByRole('textbox', { name: 'Menu icon' }),
      'UnknownIcon'
    )
    await user.click(screen.getByRole('button', { name: 'Add' }))

    expect(
      await screen.findByText('Icon not found. Enter a valid Lucide icon name.')
    ).toBeVisible()
    expect(screen.queryByTestId('lucide-unknown-icon')).not.toBeInTheDocument()
    expect(onSave).not.toHaveBeenCalled()
  })

  test('defaults an HTTP URL to embedded mode', async () => {
    const onSave = vi.fn()
    const user = userEvent.setup()
    render(
      <ChatDialog open onOpenChange={vi.fn()} onSave={onSave} editData={null} />
    )

    await user.type(
      screen.getByRole('textbox', { name: 'Chat Client Name' }),
      'Canvas'
    )
    await user.type(
      screen.getByRole('textbox', { name: 'URL' }),
      'https://canvas.example'
    )
    expect(screen.getByText('Open mode')).toBeInTheDocument()
    expect(screen.getByText('Embedded')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Add' }))

    await waitFor(() =>
      expect(onSave).toHaveBeenCalledWith({
        name: 'Canvas',
        url: 'https://canvas.example',
        openMode: 'embedded',
      })
    )
  })

  test('does not persist an open mode for custom protocols', async () => {
    const onSave = vi.fn()
    const user = userEvent.setup()
    render(
      <ChatDialog open onOpenChange={vi.fn()} onSave={onSave} editData={null} />
    )

    await user.type(
      screen.getByRole('textbox', { name: 'Chat Client Name' }),
      'Cherry Studio'
    )
    await user.type(
      screen.getByRole('textbox', { name: 'URL' }),
      'cherrystudio://import'
    )
    expect(screen.queryByText('Open mode')).not.toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Add' }))

    await waitFor(() =>
      expect(onSave).toHaveBeenCalledWith({
        name: 'Cherry Studio',
        url: 'cherrystudio://import',
        openMode: undefined,
      })
    )
  })

  test('selects explicit sandbox permissions for an embedded page', async () => {
    const onSave = vi.fn()
    const user = userEvent.setup()
    const canvasURL = `${window.location.origin}/canvas`
    render(
      <ChatDialog open onOpenChange={vi.fn()} onSave={onSave} editData={null} />
    )

    await user.type(
      screen.getByRole('textbox', { name: 'Chat Client Name' }),
      'Canvas'
    )
    await user.type(screen.getByRole('textbox', { name: 'URL' }), canvasURL)
    const permissionsSelect = screen.getByRole('combobox', {
      name: 'Embedded page permissions',
    })
    permissionsSelect.focus()
    expect(permissionsSelect).toHaveFocus()
    await user.keyboard('{Enter}')
    const originOption = screen.getByRole('option', {
      name: /Keep the original origin/,
    })
    originOption.focus()
    await user.keyboard('{Enter}')

    expect(
      screen.getByText(
        'Scripts and original-origin access together remove reliable iframe isolation.'
      )
    ).toBeVisible()
    await user.click(screen.getByRole('button', { name: 'Add' }))

    await waitFor(() =>
      expect(onSave).toHaveBeenCalledWith({
        name: 'Canvas',
        url: canvasURL,
        openMode: 'embedded',
        sandbox: [
          'allow-downloads',
          'allow-forms',
          'allow-modals',
          'allow-popups',
          'allow-popups-to-escape-sandbox',
          'allow-presentation',
          'allow-scripts',
          'allow-same-origin',
        ],
      })
    )
  })
})
