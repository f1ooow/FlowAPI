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
import { render, screen } from '@testing-library/react'
import { describe, expect, test } from 'vitest'

import type {
  ChannelMonitoringBucket,
  ChannelMonitoringChannelSummary,
} from '../../types'
import { ChannelAvailabilityRow } from '../channel-availability-row'

function bucket(
  ts: number,
  attempt_count: number,
  success_count: number,
  state: ChannelMonitoringBucket['state']
): ChannelMonitoringBucket {
  return { ts, attempt_count, success_count, state }
}

function buckets(count: number): ChannelMonitoringBucket[] {
  return Array.from({ length: count }, (_, index) =>
    bucket(index * 1800, 4, 4, 'healthy')
  )
}

const healthyChannel: ChannelMonitoringChannelSummary = {
  channel_id: 12,
  channel_name: 'FAST CODEX',
  has_data: true,
  state: 'healthy',
  availability_rate: 99.53,
  error_rate: 0.47,
  avg_latency_ms: 1840,
  attempt_count: 1204,
  success_count: 1198,
  has_cache_data: true,
  cache_hit_rate: 62.5,
  cache_engagement_rate: 40,
  cache_request_count: 1000,
  cache_signal_count: 400,
  cache_read_tokens: 50_000,
  cache_write_tokens: 4000,
  cache_input_tokens: 80_000,
  buckets: [
    bucket(0, 0, 0, 'no-data'),
    bucket(1800, 10, 10, 'healthy'),
    bucket(3600, 10, 9, 'degraded'),
    bucket(5400, 10, 2, 'down'),
  ],
}

function timelineOf(name: string) {
  return screen.getByRole('img', {
    name: new RegExp(`Availability timeline for ${name}`),
  })
}

describe('ChannelAvailabilityRow', () => {
  test('renders one timeline slot per returned bucket', () => {
    render(<ChannelAvailabilityRow channel={healthyChannel} stepMinutes={30} />)

    expect(timelineOf('FAST CODEX').childElementCount).toBe(4)
  })

  test('follows the bucket count when the range uses a finer step', () => {
    render(
      <ChannelAvailabilityRow
        channel={{ ...healthyChannel, buckets: buckets(48) }}
        stepMinutes={5}
      />
    )

    expect(timelineOf('FAST CODEX').childElementCount).toBe(48)
  })

  test('colours each slot from the state the backend assigned', () => {
    render(<ChannelAvailabilityRow channel={healthyChannel} stepMinutes={30} />)

    const states = [...timelineOf('FAST CODEX').children].map((slot) =>
      slot.getAttribute('data-state')
    )
    expect(states).toEqual(['no-data', 'healthy', 'degraded', 'down'])
  })

  test('shows placeholders instead of 0% when the channel had no attempts', () => {
    render(
      <ChannelAvailabilityRow
        channel={{
          ...healthyChannel,
          has_data: false,
          state: 'no-data',
          availability_rate: 0,
          error_rate: 0,
          avg_latency_ms: 0,
          attempt_count: 0,
          success_count: 0,
          has_cache_data: false,
          cache_hit_rate: 0,
          cache_request_count: 0,
          cache_signal_count: 0,
          buckets: [bucket(0, 0, 0, 'no-data')],
        }}
        stepMinutes={30}
      />
    )

    expect(screen.getByText('No attempts in this window')).toBeInTheDocument()
    expect(screen.queryByText('0.00%')).toBeNull()
    // Availability, latency and cache hit; attempts legitimately shows 0.
    expect(screen.getAllByText('--')).toHaveLength(3)
  })

  // Cache has its own emptiness. A channel can serve plenty of attempts while
  // its provider never reports a cache number, and 0% would read as "caching is
  // broken on this channel" instead of "there is nothing to measure".
  test('shows a placeholder for the cache hit rate when no request reported cache usage', () => {
    render(
      <ChannelAvailabilityRow
        channel={{
          ...healthyChannel,
          has_cache_data: false,
          cache_hit_rate: 0,
          cache_engagement_rate: 0,
          cache_request_count: 1200,
          cache_signal_count: 0,
        }}
        stepMinutes={30}
      />
    )

    expect(screen.getByText('99.53%')).toBeInTheDocument()
    expect(screen.getAllByText('--')).toHaveLength(1)
    expect(
      screen.getByTitle('No request reported cache usage in this window')
    ).toBeInTheDocument()
  })

  // The number is structurally higher on OpenAI than on Claude, so the row has
  // to carry that warning; without it an operator ranks channels on it.
  test('reports the cache hit rate with its caliber and its incomparability', () => {
    render(<ChannelAvailabilityRow channel={healthyChannel} stepMinutes={30} />)

    expect(screen.getByText('62.50%')).toBeInTheDocument()
    const label = screen.getByText('Cache hit')
    expect(label.getAttribute('title')).toContain(
      '400 of 1,000 settled requests'
    )
    expect(label.getAttribute('title')).toContain(
      'Not comparable between providers'
    )
  })

  test('falls back to the channel id when the channel has been deleted', () => {
    render(
      <ChannelAvailabilityRow
        channel={{ ...healthyChannel, channel_name: '' }}
        stepMinutes={30}
      />
    )

    expect(screen.getByText('#12')).toBeInTheDocument()
    expect(screen.getByText('Deleted channel')).toBeInTheDocument()
    expect(timelineOf('#12')).toBeInTheDocument()
  })

  test('reports totals as attempts rather than requests', () => {
    render(<ChannelAvailabilityRow channel={healthyChannel} stepMinutes={30} />)

    expect(screen.getByText('Attempts')).toBeInTheDocument()
    expect(
      screen.getByText('1,198 of 1,204 attempts succeeded')
    ).toBeInTheDocument()
  })
})
