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

import {
  buildRequestRuleExpr,
  parseTiersFromExpr,
  splitBillingExprAndRequestRules,
  tryParseRequestRuleExpr,
} from '../billing-expr'

const BASE_EXPR =
  'len <= 272000 ? tier("standard", p * 5 + c * 30 + cr * 0.5) : tier("long_context", p * 10 + c * 45 + cr * 1)'

describe('billing expression structured parsing', () => {
  test('rejects expressions whose pricing cannot be represented completely', () => {
    const expressions = [
      `(${BASE_EXPR}) * (param("service_tier") == "priority" || param("service_tier") == "fast" ? 2 : 1)`,
      'tier("base", max(p * 5 + c * 30, 100))',
      'tier("base", p * 5 + c * 30) * 2',
      'tier("base", p * 5 + p * 2 + c * 30)',
    ]

    for (const expression of expressions) {
      assert.deepEqual(parseTiersFromExpr(expression), [])
    }
  })

  test('keeps canonical mutually exclusive request multipliers structured', () => {
    const expression =
      `(${BASE_EXPR}) * ` +
      '(param("service_tier") == "priority" ? 2 : 1) * ' +
      '(param("service_tier") == "fast" ? 2 : 1)'

    const split = splitBillingExprAndRequestRules(expression)

    assert.equal(split.billingExpr, BASE_EXPR)
    assert.notEqual(split.requestRuleExpr, '')
    assert.equal(parseTiersFromExpr(expression).length, 2)
  })

  test('rejects quoted values that the visual editor would change to another type', () => {
    for (const value of ['123', 'true', '1e3']) {
      assert.equal(
        tryParseRequestRuleExpr(`(param("code") == "${value}" ? 2 : 1)`),
        null
      )
    }

    assert.notEqual(
      tryParseRequestRuleExpr('(param("service_tier") == "priority" ? 2 : 1)'),
      null
    )
    assert.notEqual(
      tryParseRequestRuleExpr('(param("count") == 123 ? 2 : 1)'),
      null
    )
    assert.notEqual(
      tryParseRequestRuleExpr('(param("stream") == true ? 2 : 1)'),
      null
    )

    const headerRule = '(header("x-code") == "123" ? 2 : 1)'
    const parsedHeaderRule = tryParseRequestRuleExpr(headerRule)
    assert.notEqual(parsedHeaderRule, null)
    assert.equal(buildRequestRuleExpr(parsedHeaderRule ?? []), headerRule)
  })
})
