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

import i18next from 'i18next'
import { beforeAll, describe, test } from 'vitest'

import { getServerErrorMessage } from '../handle-server-error'

beforeAll(async () => {
  await i18next.init({
    lng: 'en',
    resources: {
      en: {
        translation: {
          'Telegram binding is disabled.': 'Localized stable error',
          'Something went wrong!': 'Generic error',
        },
      },
    },
  })
})

describe('server error display', () => {
  test('prefers a stable code translation and preserves uncoded details', () => {
    const coded = Object.assign(new Error('raw server detail'), {
      code: 'TELEGRAM_BIND_DISABLED',
    })

    assert.equal(getServerErrorMessage(coded), 'Localized stable error')
    assert.equal(
      getServerErrorMessage(
        Object.assign(new Error('actionable fallback detail'), {
          code: undefined,
        })
      ),
      'actionable fallback detail'
    )
    assert.equal(
      getServerErrorMessage(new Error('internal client detail')),
      'Generic error'
    )
  })
})
