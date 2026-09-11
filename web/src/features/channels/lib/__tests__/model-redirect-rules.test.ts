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
import { describe, expect, test } from 'vitest'

import {
  findFirstModelRedirect,
  normalizeModelRedirectJson,
  parseModelRedirectRules,
  validateModelRedirectRules,
  type ModelRedirectRule,
} from '../model-redirect-rules'

describe('model redirect rules', () => {
  test('normalizes legacy object mappings into ordered exact rules', () => {
    const normalized = normalizeModelRedirectJson(
      '{"claude-opus-5":"claude-opus-4-6","claude-sonnet-5":"claude-sonnet-4-6"}'
    )

    expect(parseModelRedirectRules(normalized)).toEqual([
      {
        match_type: 'exact',
        source: 'claude-opus-5',
        target: 'claude-opus-4-6',
      },
      {
        match_type: 'exact',
        source: 'claude-sonnet-5',
        target: 'claude-sonnet-4-6',
      },
    ])
  })

  test('uses the first matching rule across all supported match types', () => {
    const rules: ModelRedirectRule[] = [
      { match_type: 'prefix', source: 'claude-', target: 'prefix-target' },
      { match_type: 'contains', source: 'opus', target: 'opus-target' },
      { match_type: 'suffix', source: '-latest', target: 'suffix-target' },
      { match_type: 'regex', source: '^gpt-[45]', target: 'regex-target' },
      { match_type: 'exact', source: 'gpt-5', target: 'exact-target' },
    ]

    expect(findFirstModelRedirect('claude-opus-5', rules)?.rule.target).toBe(
      'prefix-target'
    )
    expect(findFirstModelRedirect('other-latest', rules)?.rule.target).toBe(
      'suffix-target'
    )
    expect(findFirstModelRedirect('gpt-5', rules)?.rule.target).toBe(
      'regex-target'
    )
  })

  test('rejects malformed regex and duplicate ordered rules', () => {
    expect(
      validateModelRedirectRules(
        '[{"match_type":"regex","source":"[","target":"model"}]'
      ).valid
    ).toBe(false)
    expect(
      validateModelRedirectRules(
        '[{"match_type":"contains","source":"opus","target":"a"},{"match_type":"contains","source":"opus","target":"b"}]'
      ).valid
    ).toBe(false)
  })
})
