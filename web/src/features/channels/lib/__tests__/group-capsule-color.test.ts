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

import { createGroupCapsuleClassName } from '../group-capsule-color'

// The groups configured in production at the time of writing.
const PRODUCTION_GROUPS = [
  'CODEX',
  'Claude 官转',
  'Claude 逆向',
  'image',
  'vedio',
  '【职刻】CC 反重力',
  '【职刻】CODEX',
  '【职刻】Claude 官转',
  '【职刻】Claude 逆向综合',
  '【职刻】Gemini',
  '【职刻】Image',
  '【职刻】国产模型',
]

describe('createGroupCapsuleClassName', () => {
  test('gives each of twelve listed groups its own colour', () => {
    const capsuleClassName = createGroupCapsuleClassName(PRODUCTION_GROUPS)

    const colours = PRODUCTION_GROUPS.map(capsuleClassName)
    expect(new Set(colours).size).toBe(12)
    for (const colour of colours) {
      expect(colour).toMatch(/^bg-[a-z]+-\d{3}$/)
    }
  })

  test('assigns colours by name, not by the order of the list', () => {
    const forward = createGroupCapsuleClassName(PRODUCTION_GROUPS)
    const reversed = createGroupCapsuleClassName(
      [...PRODUCTION_GROUPS].reverse()
    )

    for (const group of PRODUCTION_GROUPS) {
      expect(reversed(group)).toBe(forward(group))
    }
  })

  test('returns the same colour every time a group is asked for', () => {
    const capsuleClassName = createGroupCapsuleClassName(PRODUCTION_GROUPS)

    expect(capsuleClassName('【职刻】CODEX')).toBe(
      capsuleClassName('【职刻】CODEX')
    )
  })

  test('ignores duplicate entries in the list', () => {
    const withDuplicate = createGroupCapsuleClassName([
      ...PRODUCTION_GROUPS,
      'CODEX',
    ])
    const plain = createGroupCapsuleClassName(PRODUCTION_GROUPS)

    for (const group of PRODUCTION_GROUPS) {
      expect(withDuplicate(group)).toBe(plain(group))
    }
  })

  // Expected fallbacks are the FNV-1a hash of the name over the palette, so
  // an unlisted group keeps its colour across reloads and list changes.
  test('falls back to a colour hashed from the name for an unlisted group', () => {
    const listed = createGroupCapsuleClassName(PRODUCTION_GROUPS)

    expect(listed('retired-group')).toBe('bg-indigo-700')
    expect(listed('RETIRED')).toBe('bg-violet-700')
  })

  test('uses the hashed colour for every group while the list is empty', () => {
    const empty = createGroupCapsuleClassName([])

    expect(empty('CODEX')).toBe('bg-emerald-700')
    expect(empty('retired-group')).toBe('bg-indigo-700')
  })

  test('keeps an unlisted group on its hashed colour whatever the list holds', () => {
    const before = createGroupCapsuleClassName(['a'])
    const after = createGroupCapsuleClassName(['a', 'b', 'c'])

    expect(before('retired-group')).toBe(after('retired-group'))
  })

  test('reuses the palette in order past twelve groups', () => {
    const thirteen = [...PRODUCTION_GROUPS, 'zzz-extra']
    const capsuleClassName = createGroupCapsuleClassName(thirteen)
    const sorted = [...thirteen].sort((a, b) => a.localeCompare(b, 'en'))

    // The thirteenth group in name order wraps round to the first colour.
    expect(capsuleClassName(sorted[12])).toBe(capsuleClassName(sorted[0]))
    expect(new Set(sorted.slice(0, 12).map(capsuleClassName)).size).toBe(12)
  })
})
