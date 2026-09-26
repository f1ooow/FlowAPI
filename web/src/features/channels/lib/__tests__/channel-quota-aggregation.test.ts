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

import { channelSchema, type Channel } from '../../types'
import {
  aggregateChannelsByTag,
  isTagAggregateRow,
  type TagRow,
} from '../channel-utils'

type QuotaFields = Partial<
  Pick<
    Channel,
    | 'today_cost_quota'
    | 'today_used_quota'
    | 'total_cost_quota'
    | 'total_used_quota'
  >
>

function channel(id: number, tag: string, quotas: QuotaFields = {}): Channel {
  return channelSchema.parse({
    id,
    type: 1,
    key: `sk-${id}`,
    status: 1,
    name: `channel-${id}`,
    tag,
    created_time: 0,
    test_time: 0,
    response_time: 0,
    balance_updated_time: 0,
    ...quotas,
  })
}

function tagRow(channels: Channel[], tag: string): TagRow {
  const row = aggregateChannelsByTag(channels).find(
    (candidate) => candidate.tag === tag
  )
  if (!row || !isTagAggregateRow(row)) {
    throw new Error(`Missing aggregate row for tag ${tag}`)
  }
  return row
}

describe('channel quota aggregation', () => {
  test('preserves the API distinction between unavailable and zero quota', () => {
    const missing = channel(1, 'primary')
    const unavailable = channel(2, 'primary', {
      today_cost_quota: null,
      total_used_quota: null,
    })
    const zero = channel(3, 'primary', {
      today_cost_quota: 0,
      total_used_quota: 0,
    })

    expect(missing.today_cost_quota).toBeNull()
    expect(missing.total_cost_quota).toBeNull()
    expect(unavailable.today_cost_quota).toBeNull()
    expect(unavailable.total_used_quota).toBeNull()
    expect(zero.today_cost_quota).toBe(0)
    expect(zero.total_used_quota).toBe(0)
  })

  test('sums every quota figure across a tag, including explicit zeros', () => {
    const row = tagRow(
      [
        channel(1, 'primary', {
          today_cost_quota: 0,
          today_used_quota: 0,
          total_cost_quota: 400,
          total_used_quota: 1000,
        }),
        channel(2, 'primary', {
          today_cost_quota: 100,
          today_used_quota: 250,
          total_cost_quota: 600,
          total_used_quota: 1500,
        }),
      ],
      'primary'
    )

    expect(row.today_cost_quota).toBe(100)
    expect(row.today_used_quota).toBe(250)
    expect(row.total_cost_quota).toBe(1000)
    expect(row.total_used_quota).toBe(2500)
  })

  test('marks only the figure a child could not report as unavailable', () => {
    const row = tagRow(
      [
        channel(1, 'primary', {
          today_cost_quota: 100,
          today_used_quota: 250,
          total_cost_quota: null,
          total_used_quota: 1000,
        }),
        channel(2, 'primary', {
          today_cost_quota: 50,
          today_used_quota: null,
          total_cost_quota: 600,
          total_used_quota: 1500,
        }),
      ],
      'primary'
    )

    expect(row.today_cost_quota).toBe(150)
    expect(row.today_used_quota).toBeNull()
    expect(row.total_cost_quota).toBeNull()
    expect(row.total_used_quota).toBe(2500)
  })

  test('keeps a tag unavailable wherever the unavailable child sits', () => {
    const channels = [
      channel(1, 'first-null', { today_used_quota: null }),
      channel(2, 'first-null', { today_used_quota: 250 }),
      channel(3, 'last-null', { today_used_quota: 125 }),
      channel(4, 'last-null', { today_used_quota: null }),
    ]

    expect(tagRow(channels, 'first-null').today_used_quota).toBeNull()
    expect(tagRow(channels, 'last-null').today_used_quota).toBeNull()
    expect(aggregateChannelsByTag([])).toEqual([])
  })
})
