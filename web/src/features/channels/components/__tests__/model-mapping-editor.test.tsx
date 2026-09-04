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
import { fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { describe, expect, test, vi } from 'vitest'

import { ModelMappingEditor } from '../model-mapping-editor'

describe('model mapping editor', () => {
  test('shows mappings by default and lets an administrator hide one', async () => {
    const onVisibilityChange = vi.fn()
    const user = userEvent.setup()
    render(
      <ModelMappingEditor
        value='{"request-model":"upstream-model"}'
        onChange={() => undefined}
        userHiddenModelMappings={[]}
        onUserHiddenModelMappingsChange={onVisibilityChange}
      />
    )

    const checkbox = await screen.findByRole('checkbox', {
      name: 'User Visible: request-model',
    })
    expect(checkbox).toBeChecked()
    checkbox.focus()
    await user.keyboard(' ')

    expect(onVisibilityChange).toHaveBeenCalledWith(['request-model'])
  })

  test('renaming a hidden source creates a visible mapping', async () => {
    function Harness() {
      const [value, setValue] = useState('{"request-model":"upstream-model"}')
      const [hidden, setHidden] = useState(['request-model'])
      return (
        <ModelMappingEditor
          value={value}
          onChange={setValue}
          userHiddenModelMappings={hidden}
          onUserHiddenModelMappingsChange={setHidden}
        />
      )
    }
    render(<Harness />)

    const source = await screen.findByDisplayValue('request-model')
    fireEvent.change(source, { target: { value: 'renamed-model' } })

    expect(
      screen.getByRole('checkbox', {
        name: 'User Visible: renamed-model',
      })
    ).toBeChecked()

    fireEvent.click(screen.getByRole('button', { name: 'Add Mapping' }))
    const sourceInputs = screen.getAllByPlaceholderText('gpt-3.5-turbo')
    const newSource = sourceInputs.at(-1)
    if (!newSource) throw new Error('new mapping source input not found')
    fireEvent.change(newSource, {
      target: { value: 'request-model' },
    })

    expect(
      screen.getByRole('checkbox', {
        name: 'User Visible: request-model',
      })
    ).toBeChecked()
  })

  test('re-adding a source removed in JSON mode creates a visible mapping', async () => {
    function Harness() {
      const [value, setValue] = useState('{"request-model":"upstream-model"}')
      const [hidden, setHidden] = useState(['request-model'])
      return (
        <ModelMappingEditor
          value={value}
          onChange={setValue}
          userHiddenModelMappings={hidden}
          onUserHiddenModelMappingsChange={setHidden}
        />
      )
    }
    render(<Harness />)

    fireEvent.click(await screen.findByRole('tab', { name: 'JSON' }))
    fireEvent.input(screen.getByLabelText('Model Mapping'), {
      target: { value: '{"renamed-model":"upstream-model"}' },
    })
    fireEvent.click(screen.getByRole('tab', { name: 'Visual' }))
    fireEvent.click(screen.getByRole('button', { name: 'Add Mapping' }))

    const sourceInputs = screen.getAllByPlaceholderText('gpt-3.5-turbo')
    const newSource = sourceInputs.at(-1)
    if (!newSource) throw new Error('new mapping source input not found')
    fireEvent.change(newSource, {
      target: { value: 'request-model' },
    })

    expect(
      screen.getByRole('checkbox', {
        name: 'User Visible: request-model',
      })
    ).toBeChecked()
  })

  test('disables user visibility when sensitive channel settings are locked', async () => {
    const onVisibilityChange = vi.fn()
    const user = userEvent.setup()
    render(
      <ModelMappingEditor
        value='{"request-model":"upstream-model"}'
        onChange={() => undefined}
        userHiddenModelMappings={[]}
        onUserHiddenModelMappingsChange={onVisibilityChange}
        userHiddenModelMappingsDisabled
      />
    )

    const checkbox = await screen.findByRole('checkbox', {
      name: 'User Visible: request-model',
    })
    expect(checkbox).toHaveAttribute('aria-disabled', 'true')
    await user.click(checkbox)
    expect(onVisibilityChange).not.toHaveBeenCalled()
  })

  test('keeps hidden settings unchanged when mappings change while sensitive settings are locked', async () => {
    const onVisibilityChange = vi.fn()
    render(
      <ModelMappingEditor
        value='{"request-model":"upstream-model"}'
        onChange={() => undefined}
        userHiddenModelMappings={['request-model']}
        onUserHiddenModelMappingsChange={onVisibilityChange}
        userHiddenModelMappingsDisabled
      />
    )

    fireEvent.change(await screen.findByDisplayValue('request-model'), {
      target: { value: 'renamed-model' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Delete mapping' }))

    expect(onVisibilityChange).not.toHaveBeenCalled()
  })
})
