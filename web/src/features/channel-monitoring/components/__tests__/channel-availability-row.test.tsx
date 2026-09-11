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
          buckets: [bucket(0, 0, 0, 'no-data')],
        }}
        stepMinutes={30}
      />
    )

    expect(screen.getByText('No attempts in this window')).toBeInTheDocument()
    expect(screen.queryByText('0.00%')).toBeNull()
    expect(screen.getAllByText('--')).toHaveLength(2)
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
