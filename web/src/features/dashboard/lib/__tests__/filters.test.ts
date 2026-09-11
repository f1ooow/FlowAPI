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
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'

import { DEFAULT_DASHBOARD_CHART_PREFERENCES } from '../../constants'
import { buildDefaultDashboardFilters } from '../filters'

beforeEach(() => {
  vi.useFakeTimers()
})

afterEach(() => {
  vi.useRealTimers()
})

describe('dashboard default filters', () => {
  test('uses China midnight for the default one-day range', () => {
    vi.setSystemTime(new Date('2026-09-04T10:37:42Z'))

    const filters = buildDefaultDashboardFilters(
      DEFAULT_DASHBOARD_CHART_PREFERENCES
    )

    expect(filters.start_timestamp?.toISOString()).toBe(
      '2026-09-03T16:00:00.000Z'
    )
    expect(filters.end_timestamp?.toISOString()).toBe(
      '2026-09-04T10:37:42.000Z'
    )
  })

  test('keeps explicitly selected longer ranges rolling', () => {
    vi.setSystemTime(new Date('2026-09-04T10:37:42Z'))

    const filters = buildDefaultDashboardFilters({
      ...DEFAULT_DASHBOARD_CHART_PREFERENCES,
      defaultTimeRangeDays: 7,
    })

    expect(filters.start_timestamp?.toISOString()).toBe(
      '2026-08-28T10:37:42.000Z'
    )
    expect(filters.end_timestamp?.toISOString()).toBe(
      '2026-09-04T10:37:42.000Z'
    )
  })
})
