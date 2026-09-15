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
import { aggregateChannelsByTag, isTagAggregateRow } from '../channel-utils'

function channel(
  id: number,
  tag: string,
  todayUsedQuota?: number | null
): Channel {
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
    today_used_quota: todayUsedQuota,
  })
}

function tagQuota(channels: Channel[], tag: string): number | null {
  const row = aggregateChannelsByTag(channels).find(
    (candidate) => candidate.tag === tag
  )
  if (!row || !isTagAggregateRow(row)) {
    throw new Error(`Missing aggregate row for tag ${tag}`)
  }
  return row.today_used_quota
}

describe('channel daily consumption', () => {
  test('preserves the API distinction between unavailable and zero usage', () => {
    expect(channel(1, 'primary').today_used_quota).toBeNull()
    expect(channel(2, 'primary', null).today_used_quota).toBeNull()
    expect(channel(3, 'primary', 0).today_used_quota).toBe(0)
  })

  test('sums available child values, including an explicit zero', () => {
    const channels = [channel(1, 'primary', 0), channel(2, 'primary', 250)]

    expect(tagQuota(channels, 'primary')).toBe(250)
  })

  test('marks a tag unavailable when any child value is unavailable', () => {
    const channels = [
      channel(1, 'first-null', null),
      channel(2, 'first-null', 250),
      channel(3, 'last-null', 125),
      channel(4, 'last-null', null),
    ]

    expect(tagQuota(channels, 'first-null')).toBeNull()
    expect(tagQuota(channels, 'last-null')).toBeNull()
    expect(aggregateChannelsByTag([])).toEqual([])
  })
})
