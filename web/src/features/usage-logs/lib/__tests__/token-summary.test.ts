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

import type { UsageLog } from '../../data/schema'
import { getLogTokenSummary } from '../format'

const baseLog = {
  prompt_tokens: 0,
  completion_tokens: 0,
} as UsageLog

describe('usage log token summary', () => {
  test('adds cache components to Anthropic total input', () => {
    const summary = getLogTokenSummary(
      { ...baseLog, prompt_tokens: 70, completion_tokens: 7 },
      {
        usage_semantic: 'anthropic',
        cache_tokens: 30,
        cache_creation_tokens: 20,
        cache_creation_tokens_5m: 12,
        cache_creation_tokens_1h: 8,
      }
    )

    assert.deepEqual(summary, {
      inputTotal: 120,
      outputTotal: 7,
      cacheRead: 30,
      cacheWrite: 20,
      cacheWrite5m: 12,
      cacheWrite1h: 8,
      cacheWriteUnclassified: 0,
      cacheTotal: 50,
      uncachedInput: 70,
      inputBreakdownValid: true,
    })
  })

  test('does not add cache already included by OpenAI', () => {
    const summary = getLogTokenSummary(
      { ...baseLog, prompt_tokens: 120, completion_tokens: 7 },
      {
        usage_semantic: 'openai',
        cache_tokens: 30,
        cache_creation_tokens: 20,
      }
    )

    assert.equal(summary.inputTotal, 120)
  })

  test('prefers an explicitly normalized total input', () => {
    const summary = getLogTokenSummary(
      { ...baseLog, prompt_tokens: 70 },
      {
        usage_semantic: 'anthropic',
        input_tokens_total: 100,
        cache_tokens: 30,
      }
    )

    assert.equal(summary.inputTotal, 100)
  })

  test('does not add cache to a migrated prompt total', () => {
    const summary = getLogTokenSummary(
      { ...baseLog, prompt_tokens: 100 },
      {
        usage_semantic: 'anthropic',
        input_tokens_total: 100,
        cache_tokens: 30,
      }
    )

    assert.equal(summary.inputTotal, 100)
  })

  test('uses the legacy Claude marker only when semantic is absent', () => {
    const legacy = getLogTokenSummary(
      { ...baseLog, prompt_tokens: 70 },
      { claude: true, cache_tokens: 30 }
    )
    const explicitOpenAI = getLogTokenSummary(
      { ...baseLog, prompt_tokens: 100 },
      { claude: true, usage_semantic: 'openai', cache_tokens: 30 }
    )

    assert.equal(legacy.inputTotal, 100)
    assert.equal(explicitOpenAI.inputTotal, 100)
  })

  test('keeps cache write TTL values nested without double counting', () => {
    const summary = getLogTokenSummary(
      { ...baseLog, prompt_tokens: 140 },
      {
        usage_semantic: 'openai',
        cache_tokens: 30,
        cache_write_tokens: 50,
        cache_creation_tokens_5m: 20,
        cache_creation_tokens_1h: 10,
      }
    )

    assert.equal(summary.cacheWrite, 50)
    assert.equal(summary.cacheWrite5m, 20)
    assert.equal(summary.cacheWrite1h, 10)
    assert.equal(summary.cacheWriteUnclassified, 20)
    assert.equal(summary.cacheTotal, 80)
    assert.equal(summary.uncachedInput, 60)
    assert.equal(summary.inputBreakdownValid, true)
  })

  test('marks inconsistent historical cache values instead of inventing input composition', () => {
    const summary = getLogTokenSummary(
      { ...baseLog, prompt_tokens: 40 },
      {
        usage_semantic: 'openai',
        cache_tokens: 50,
        cache_creation_tokens: 20,
      }
    )

    assert.equal(summary.cacheTotal, 70)
    assert.equal(summary.uncachedInput, 0)
    assert.equal(summary.inputBreakdownValid, false)
  })
})
