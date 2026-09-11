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
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import { ChannelMonitoring } from '..'
import type {
  ChannelMonitoringChannelSummary,
  ChannelMonitoringSummary,
} from '../types'

function channel(
  overrides: Partial<ChannelMonitoringChannelSummary>
): ChannelMonitoringChannelSummary {
  return {
    channel_id: 1,
    channel_name: 'Primary',
    has_data: true,
    state: 'healthy',
    availability_rate: 100,
    error_rate: 0,
    avg_latency_ms: 900,
    attempt_count: 40,
    success_count: 40,
    buckets: [
      { ts: 0, attempt_count: 40, success_count: 40, state: 'healthy' },
    ],
    ...overrides,
  }
}

function summary(range: string): ChannelMonitoringSummary {
  return {
    range,
    ranges: ['15m', '1h', '6h', '24h', '7d'],
    step_minutes: range === '7d' ? 120 : 30,
    window_seconds: 86400,
    overall: {
      has_data: true,
      availability_rate: 100,
      error_rate: 0,
      avg_latency_ms: 900,
      attempt_count: 40,
    },
    channels: [
      channel({}),
      channel({
        channel_id: 2,
        channel_name: 'Idle backup',
        has_data: false,
        state: 'no-data',
        availability_rate: 0,
        avg_latency_ms: 0,
        attempt_count: 0,
        success_count: 0,
        buckets: [
          { ts: 0, attempt_count: 0, success_count: 0, state: 'no-data' },
        ],
      }),
    ],
  }
}

function renderPage() {
  const requestedRanges: string[] = []
  vi.spyOn(api, 'get').mockImplementation(async (_url, config) => {
    const range = String(
      (config?.params as Record<string, unknown> | undefined)?.range ?? ''
    )
    requestedRanges.push(range)
    return { data: { success: true, data: summary(range) } } as never
  })

  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={queryClient}>
      <ChannelMonitoring />
    </QueryClientProvider>
  )
  return requestedRanges
}

describe('ChannelMonitoring page', () => {
  test('requests the default 24h window and lists every channel', async () => {
    const requestedRanges = renderPage()

    await waitFor(() => {
      expect(screen.getByText('Primary')).toBeInTheDocument()
    })
    expect(requestedRanges).toEqual(['24h'])
    expect(screen.getByText('Idle backup')).toBeInTheDocument()
  })

  test('refetches with the range the API advertised when one is picked', async () => {
    const requestedRanges = renderPage()
    await waitFor(() => {
      expect(screen.getByText('Primary')).toBeInTheDocument()
    })

    await userEvent.click(screen.getByRole('button', { name: '7d' }))

    await waitFor(() => {
      expect(requestedRanges).toContain('7d')
    })
    expect(screen.getByRole('button', { name: '7d' })).toHaveAttribute(
      'aria-pressed',
      'true'
    )
  })

  test('hides channels without traffic once the filter is switched on', async () => {
    renderPage()
    await waitFor(() => {
      expect(screen.getByText('Idle backup')).toBeInTheDocument()
    })

    await userEvent.click(
      screen.getByRole('switch', { name: 'Hide channels without traffic' })
    )

    await waitFor(() => {
      expect(screen.queryByText('Idle backup')).toBeNull()
    })
    expect(screen.getByText('Primary')).toBeInTheDocument()
  })
})
