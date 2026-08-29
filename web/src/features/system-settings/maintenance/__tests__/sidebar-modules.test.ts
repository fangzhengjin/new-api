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
import { expect, test } from 'vitest'

import { parseSidebarModulesAdmin } from '../config'

test('legacy sidebar config enables the three wallet sections by default', () => {
  const config = parseSidebarModulesAdmin(
    JSON.stringify({ personal: { enabled: true, topup: true } })
  )

  expect(config.personal.wallet_add_funds).toBe(true)
  expect(config.personal.wallet_subscriptions).toBe(true)
  expect(config.personal.wallet_affiliate).toBe(true)
})

test('explicit wallet section visibility remains unchanged', () => {
  const config = parseSidebarModulesAdmin(
    JSON.stringify({
      personal: {
        enabled: true,
        topup: true,
        wallet_add_funds: false,
        wallet_subscriptions: false,
        wallet_affiliate: false,
      },
    })
  )

  expect(config.personal.wallet_add_funds).toBe(false)
  expect(config.personal.wallet_subscriptions).toBe(false)
  expect(config.personal.wallet_affiliate).toBe(false)
})
