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

import type { ChannelMonitoringOverall } from '../../types'
import { MonitoringOverviewCards } from '../monitoring-overview-cards'

const noCache = {
  has_cache_data: false,
  cache_hit_rate: 0,
  cache_engagement_rate: 0,
  cache_request_count: 0,
  cache_signal_count: 0,
  cache_read_tokens: 0,
  cache_write_tokens: 0,
  cache_input_tokens: 0,
}

const withTraffic: ChannelMonitoringOverall = {
  has_data: true,
  availability_rate: 97.46,
  error_rate: 2.54,
  avg_latency_ms: 68940,
  attempt_count: 5120,
  ...noCache,
}

describe('MonitoringOverviewCards', () => {
  test('shows availability, latency and error rate from the window totals', () => {
    render(<MonitoringOverviewCards overall={withTraffic} />)

    expect(screen.getByText('97.46%')).toBeInTheDocument()
    expect(screen.getByText('2.54%')).toBeInTheDocument()
    expect(screen.getByText('68.9 s')).toBeInTheDocument()
  })

  test('renders placeholders instead of zeroes when no attempt was recorded', () => {
    render(
      <MonitoringOverviewCards
        overall={{
          has_data: false,
          availability_rate: 0,
          error_rate: 0,
          avg_latency_ms: 0,
          attempt_count: 0,
          ...noCache,
        }}
      />
    )

    expect(screen.getAllByText('--')).toHaveLength(4)
    expect(screen.queryByText('0.00%')).toBeNull()
    expect(
      screen.getAllByText('No attempts in this window').length
    ).toBeGreaterThan(0)
  })

  // Cache emptiness is independent of attempt emptiness: this window is full of
  // attempts and still has nothing to measure, so the card must not claim 0%.
  test('keeps the cache card empty while attempts are present but no cache was reported', () => {
    render(<MonitoringOverviewCards overall={withTraffic} />)

    expect(screen.getByText('97.46%')).toBeInTheDocument()
    expect(screen.getAllByText('--')).toHaveLength(1)
    expect(
      screen.getByText('No request reported cache usage in this window')
    ).toBeInTheDocument()
  })

  test('labels the cache hit rate as not comparable between providers', () => {
    render(
      <MonitoringOverviewCards
        overall={{
          ...withTraffic,
          has_cache_data: true,
          cache_hit_rate: 61.25,
          cache_signal_count: 812,
          cache_request_count: 5000,
          cache_read_tokens: 49_000,
          cache_input_tokens: 80_000,
        }}
      />
    )

    expect(screen.getByText('61.25%')).toBeInTheDocument()
    expect(
      screen.getByText(
        'Cache reads over input tokens across the 812 settled requests that reported cache usage. Not comparable between providers.'
      )
    ).toBeInTheDocument()
  })
})
