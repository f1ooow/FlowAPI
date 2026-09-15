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
  today_used_quota: 1_250_000,
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
    render(<ChannelAvailabilityRow channel={healthyChannel} />)

    expect(timelineOf('FAST CODEX').childElementCount).toBe(4)
  })

  test('keeps metrics in two columns so compact cards do not truncate values', () => {
    render(<ChannelAvailabilityRow channel={healthyChannel} />)

    const metrics = screen.getByText('Availability').closest('dl')
    expect(metrics).toHaveClass('grid-cols-2')
    expect(screen.getByText('99.53%')).not.toHaveClass('truncate')
    expect(screen.getByText('1840 ms')).not.toHaveClass('truncate')
  })

  test('shows today consumption as a full-width formatted metric', () => {
    render(<ChannelAvailabilityRow channel={healthyChannel} />)

    const label = screen.getByText('Consumed today')
    expect(label.parentElement).toHaveClass('col-span-2', 'border-t')
    expect(screen.getByText('$2.5')).toHaveAttribute('title', '$2.5')
  })

  test('shows a placeholder when today consumption is unavailable', () => {
    render(
      <ChannelAvailabilityRow
        channel={{ ...healthyChannel, today_used_quota: null }}
      />
    )

    const label = screen.getByText('Consumed today')
    expect(label.parentElement).toHaveTextContent('--')
  })

  test('follows the bucket count when the range uses a finer step', () => {
    render(
      <ChannelAvailabilityRow
        channel={{ ...healthyChannel, buckets: buckets(48) }}
      />
    )

    expect(timelineOf('FAST CODEX').childElementCount).toBe(48)
  })

  test('colours each slot from the state the backend assigned', () => {
    render(<ChannelAvailabilityRow channel={healthyChannel} />)

    const states = [...timelineOf('FAST CODEX').children].map((slot) =>
      slot.getAttribute('data-state')
    )
    expect(states).toEqual(['no-data', 'healthy', 'degraded', 'down'])
  })

  test('keeps exact bucket results in the timeline tooltip', () => {
    render(<ChannelAvailabilityRow channel={healthyChannel} />)

    const healthySlot = timelineOf('FAST CODEX').children.item(1)
    expect(healthySlot).toHaveAttribute(
      'title',
      expect.stringContaining('100% (10/10)')
    )
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
      />
    )

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
      />
    )

    expect(screen.getByText('99.53%')).toBeInTheDocument()
    expect(screen.getAllByText('--')).toHaveLength(1)
    expect(screen.getByText('Cache hit')).not.toHaveAttribute('title')
  })

  test('shows the reported cache hit rate without explanatory copy', () => {
    render(<ChannelAvailabilityRow channel={healthyChannel} />)

    expect(screen.getByText('62.50%')).toBeInTheDocument()
    const label = screen.getByText('Cache hit')
    expect(label).not.toHaveAttribute('title')
    expect(screen.queryByText(/Not comparable between providers/)).toBeNull()
  })

  test('falls back to the channel id when the channel has been deleted', () => {
    render(
      <ChannelAvailabilityRow
        channel={{ ...healthyChannel, channel_name: '' }}
      />
    )

    expect(screen.getByText('#12')).toBeInTheDocument()
    expect(screen.getByText('Deleted channel')).toBeInTheDocument()
    expect(timelineOf('#12')).toBeInTheDocument()
  })

  test('reports totals as attempts rather than requests', () => {
    render(<ChannelAvailabilityRow channel={healthyChannel} />)

    expect(screen.getByText('Attempts')).toBeInTheDocument()
    expect(screen.getByText('1,204')).toBeInTheDocument()
    expect(screen.queryByText(/attempts succeeded/)).toBeNull()
    expect(screen.queryByText(/min per bar/)).toBeNull()
  })
})
