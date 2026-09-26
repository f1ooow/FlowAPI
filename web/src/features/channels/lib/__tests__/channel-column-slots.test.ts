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
import { afterEach, describe, expect, test } from 'vitest'

import {
  DEFAULT_CURRENCY_CONFIG,
  useSystemConfigStore,
} from '@/stores/system-config-store'

import { channelSchema, type Channel } from '../../types'
import { getChannelColumnSlotWidths } from '../channel-column-slots'
import { aggregateChannelsByTag } from '../channel-utils'

// 500_000 quota is one USD under the default currency settings.
const USD = 500_000
const VISIBLE = { sensitiveVisible: true, showSymbol: true, locale: 'en-US' }

function channel(id: number, fields: Partial<Channel>): Channel {
  return channelSchema.parse({
    id,
    type: 1,
    key: `sk-${id}`,
    status: 1,
    name: `channel-${id}`,
    created_time: 0,
    test_time: 0,
    response_time: 0,
    balance_updated_time: 0,
    ...fields,
  })
}

afterEach(() => {
  useSystemConfigStore.getState().setConfig({
    currency: { ...DEFAULT_CURRENCY_CONFIG },
  })
})

describe('getChannelColumnSlotWidths', () => {
  test('sizes each amount slot to the longest text on the page', () => {
    const rows = [
      // $1.10 / $1.57 / $0.47
      channel(1, {
        today_cost_quota: 1.1 * USD,
        today_used_quota: 1.57 * USD,
      }),
      // $2,823.47 / $3,000.00 / $176.53
      channel(2, {
        today_cost_quota: 2823.47 * USD,
        today_used_quota: 3000 * USD,
      }),
    ]

    const widths = getChannelColumnSlotWidths(rows, 'today', VISIBLE)

    expect(widths.cost).toBe('$2,823.47'.length)
    expect(widths.used).toBe('$3,000.00'.length)
    expect(widths.profit).toBe('$176.53'.length)
  })

  test('counts the minus sign of a loss', () => {
    const rows = [
      channel(1, { today_cost_quota: 20 * USD, today_used_quota: 5 * USD }),
    ]

    expect(getChannelColumnSlotWidths(rows, 'today', VISIBLE).profit).toBe(
      '-$15.00'.length
    )
  })

  test('reads the all-time figures for the total scope', () => {
    const rows = [
      channel(1, {
        today_cost_quota: 1 * USD,
        today_used_quota: 2 * USD,
        total_cost_quota: 12_345 * USD,
        total_used_quota: 23_456 * USD,
      }),
    ]

    const today = getChannelColumnSlotWidths(rows, 'today', VISIBLE)
    const total = getChannelColumnSlotWidths(rows, 'total', VISIBLE)

    expect(today.used).toBe('$2.00'.length)
    expect(total.used).toBe('$23,456.00'.length)
  })

  test('sizes masked and unavailable amounts by what they show', () => {
    const rows = [channel(1, { today_used_quota: 1234 * USD })]

    const masked = getChannelColumnSlotWidths(rows, 'today', {
      ...VISIBLE,
      sensitiveVisible: false,
    })

    expect(masked.used).toBe('••••'.length)
    // The cost is unavailable, so the slot holds "--".
    expect(masked.cost).toBe('--'.length)
  })

  test('sizes the routing chips to the longest priority, weight and ratio', () => {
    const rows = [
      channel(1, { priority: 5, weight: 1, cost_ratio: 0.2 }),
      channel(2, { priority: 100, weight: 25, cost_ratio: 12.5 }),
      channel(3, { priority: -10, weight: 0, cost_ratio: null }),
    ]

    const widths = getChannelColumnSlotWidths(rows, 'today', VISIBLE)

    expect(widths.priority).toBe('100'.length)
    expect(widths.weight).toBe('25'.length)
    expect(widths.costRatio).toBe('12.50x'.length)
  })

  test('includes tag rows and the channels inside them', () => {
    const [tagRow] = aggregateChannelsByTag([
      channel(1, { tag: 'pool', cost_ratio: 0.2, priority: 1 }),
      channel(2, { tag: 'pool', cost_ratio: 0.5, priority: 2000 }),
    ])

    const widths = getChannelColumnSlotWidths([tagRow], 'today', VISIBLE)

    // The tag shows its ratio range; a child shows the longest priority.
    expect(widths.costRatio).toBe('0.20x-0.50x'.length)
    expect(widths.priority).toBe('2000'.length)
  })

  test('counts a full-width currency symbol as two columns', () => {
    useSystemConfigStore.getState().setConfig({
      currency: {
        ...DEFAULT_CURRENCY_CONFIG,
        quotaDisplayType: 'CUSTOM',
        customCurrencySymbol: '元',
        customCurrencyExchangeRate: 1,
      },
    })
    const rows = [
      channel(1, { today_used_quota: 1 * USD, today_cost_quota: 0 }),
    ]

    const widths = getChannelColumnSlotWidths(rows, 'today', VISIBLE)

    // "元 1.00": the wide symbol takes two columns, then a space and 1.00.
    expect(widths.used).toBe(2 + ' 1.00'.length)
  })

  test('returns zero widths for an empty page', () => {
    expect(getChannelColumnSlotWidths([], 'today', VISIBLE)).toEqual({
      cost: 0,
      used: 0,
      profit: 0,
      priority: 0,
      weight: 0,
      costRatio: 0,
    })
  })
})
