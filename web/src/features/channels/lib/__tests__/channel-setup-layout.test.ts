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

import { describe, test } from 'vitest'

import { channelSetupLayoutClasses } from '../channel-discovery-form'

describe('channel setup responsive layout', () => {
  test('stacks the workspace until extra-large screens have room for both panels', () => {
    const classes = channelSetupLayoutClasses.workspace.split(' ')

    assert.ok(classes.includes('grid'))
    assert.ok(
      classes.includes('xl:grid-cols-[minmax(18rem,22rem)_minmax(0,1fr)]')
    )
    assert.equal(
      classes.some((value) => value.startsWith('grid-cols-')),
      false
    )
  })

  test('uses the standard page width and responsive footer layout', () => {
    assert.ok(channelSetupLayoutClasses.content.includes('max-w-7xl'))
    assert.ok(channelSetupLayoutClasses.footer.includes('flex-col'))
    assert.ok(channelSetupLayoutClasses.footer.includes('sm:flex-row'))
  })

  test('visually distinguishes the selected discovery result', () => {
    assert.match(channelSetupLayoutClasses.discoveryResult, /border-primary/)
    assert.match(channelSetupLayoutClasses.discoveryResult, /bg-muted/)
  })

  test('keeps primary mobile actions at least 44 pixels high', () => {
    assert.ok(
      channelSetupLayoutClasses.mobileAction
        .split(' ')
        .includes('max-sm:min-h-11')
    )
  })

  test('raises mobile form controls and tab targets to 44 pixels', () => {
    assert.match(channelSetupLayoutClasses.mobileControls, /slot=input.*h-11/)
    assert.match(
      channelSetupLayoutClasses.mobileControls,
      /slot=native-select.*h-11/
    )
    assert.match(channelSetupLayoutClasses.mobileControls, /tabs-list.*h-11/)
  })
})
