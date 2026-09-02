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
import i18next from 'i18next'
import { expect, test } from 'vitest'

import en from '../en.json'
import fr from '../fr.json'
import ja from '../ja.json'
import ru from '../ru.json'
import vi from '../vi.json'
import zhTW from '../zh-TW.json'
import zh from '../zh.json'

const keys = [
  'Allow {{ip}} as the current source? The normal association expires after {{hours}} hours without activity and is not a permanent whitelist.',
  'IP switch wait has ended; retrying will be checked against association limits',
  'IP switch wait only controls switching; association limits are checked separately',
] as const

test.each([
  ['en', en],
  ['zh', zh],
  ['zh-TW', zhTW],
  ['fr', fr],
  ['ja', ja],
  ['ru', ru],
  ['vi', vi],
])(
  '%s keeps access-source copy in the runtime translation namespace',
  (_, locale) => {
    for (const key of keys) {
      expect(locale.translation[key]).toBeTruthy()
    }
  }
)

test('zh runtime resolves and interpolates the allowed-source explanation', async () => {
  const instance = i18next.createInstance()
  await instance.init({ lng: 'zh', resources: { zh } })

  expect(instance.t(keys[0], { ip: '192.0.2.1', hours: 24 })).toBe(
    '允许 192.0.2.1 成为当前来源？正常关联会在连续 24 小时无活动后失效，不是永久白名单'
  )
})
