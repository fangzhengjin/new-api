import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'

import { RULE_TEMPLATES } from '../constants'
import { RuleEditorDialog } from '../rule-editor-dialog'

describe('channel affinity templates', () => {
  it('keeps built-in client headers independent from affinity', () => {
    expect(RULE_TEMPLATES.codexCli.param_override_template).toBeUndefined()
    expect(RULE_TEMPLATES.claudeCli.param_override_template).toBeUndefined()
  })

  it('fills an example only after the user selects it', async () => {
    render(
      <RuleEditorDialog
        open
        onOpenChange={vi.fn()}
        rule={null}
        onSave={vi.fn()}
      />
    )

    fireEvent.click(screen.getByRole('button', { name: /Advanced Settings/ }))
    const editor = screen.getByRole('textbox', {
      name: 'Parameter Override Template (JSON)',
    })
    expect(editor).toHaveValue('')

    fireEvent.click(screen.getByRole('combobox', { name: 'Example' }))
    fireEvent.click(screen.getByRole('option', { name: 'Set Field' }))

    await waitFor(() => {
      expect(editor).toHaveValue(
        JSON.stringify(
          {
            operations: [
              { mode: 'set', path: 'service_tier', value: 'priority' },
            ],
          },
          null,
          2
        )
      )
    })
  })
})
