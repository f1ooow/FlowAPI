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
*/
import { describe, expect, test } from 'vitest'

import { getChinaTodayTimestampRange } from './time'

describe('China Standard Time day range', () => {
  test('starts at China midnight before the UTC+8 day boundary', () => {
    const now = new Date('2026-09-04T15:59:59.900Z')

    expect(getChinaTodayTimestampRange(now)).toEqual({
      start_timestamp: Date.parse('2026-09-03T16:00:00Z') / 1000,
      end_timestamp: Date.parse('2026-09-04T15:59:59Z') / 1000,
    })
  })

  test('moves the start to the new day at China midnight', () => {
    const now = new Date('2026-09-04T16:00:00Z')

    expect(getChinaTodayTimestampRange(now)).toEqual({
      start_timestamp: Date.parse('2026-09-04T16:00:00Z') / 1000,
      end_timestamp: Date.parse('2026-09-04T16:00:00Z') / 1000,
    })
  })
})
