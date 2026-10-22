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
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import { ChannelMonitoring } from '..'
import type {
  ChannelMonitoringChannelSummary,
  ChannelMonitoringSummary,
  ChannelMonitoringTotalsResponse,
  ChannelMonitoringTotalsWindow,
} from '../types'

function channel(
  overrides: Partial<ChannelMonitoringChannelSummary>
): ChannelMonitoringChannelSummary {
  return {
    channel_id: 1,
    channel_name: 'Primary',
    groups: ['default'],
    today_used_quota: 500_000,
    today_cost_quota: 200_000,
    has_data: true,
    state: 'healthy',
    availability_rate: 100,
    error_rate: 0,
    avg_latency_ms: 900,
    attempt_count: 40,
    success_count: 40,
    has_cache_data: true,
    cache_hit_rate: 62.5,
    cache_engagement_rate: 50,
    cache_request_count: 40,
    cache_signal_count: 20,
    cache_read_tokens: 5000,
    cache_write_tokens: 400,
    cache_input_tokens: 8000,
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
      has_cache_data: true,
      cache_hit_rate: 62.5,
      cache_engagement_rate: 50,
      cache_request_count: 40,
      cache_signal_count: 20,
      cache_read_tokens: 5000,
      cache_write_tokens: 400,
      cache_input_tokens: 8000,
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
        has_cache_data: false,
        cache_hit_rate: 0,
        cache_engagement_rate: 0,
        cache_request_count: 0,
        cache_signal_count: 0,
        cache_read_tokens: 0,
        cache_write_tokens: 0,
        cache_input_tokens: 0,
        buckets: [
          { ts: 0, attempt_count: 0, success_count: 0, state: 'no-data' },
        ],
      }),
    ],
  }
}

function totals(overrides: Partial<ChannelMonitoringTotalsWindow> = {}) {
  const window: ChannelMonitoringTotalsWindow = {
    used_quota: 5_000_000,
    cost_quota: 2_000_000,
    profit: 3_000_000,
    cost_has_estimate: false,
    ...overrides,
  }
  return {
    available: true as const,
    totals: { today: window, month: window, total: window },
  }
}

function renderPage(
  options: {
    totals?: ChannelMonitoringTotalsResponse
    channels?: ChannelMonitoringChannelSummary[]
  } = {}
) {
  const requestedRanges: string[] = []
  vi.spyOn(api, 'get').mockImplementation(async (url, config) => {
    if (String(url).includes('/totals')) {
      return {
        data: { success: true, data: options.totals ?? totals() },
      } as never
    }
    const range = String(
      (config?.params as Record<string, unknown> | undefined)?.range ?? ''
    )
    requestedRanges.push(range)
    const data = summary(range)
    if (options.channels) data.channels = options.channels
    return { data: { success: true, data } } as never
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
  test('requests the default 24h window and excludes channels without traffic', async () => {
    const requestedRanges = renderPage()

    await waitFor(() => {
      expect(screen.getByText('Primary')).toBeInTheDocument()
    })
    expect(requestedRanges).toEqual(['24h'])
    expect(screen.queryByText('Idle backup')).toBeNull()
  })

  test('omits the redundant fleet-wide health cards while retaining channel-level health', async () => {
    renderPage()

    expect(await screen.findByText('Primary')).toBeInTheDocument()
    expect(screen.queryByText('Overall availability')).toBeNull()
    expect(screen.queryByText('Average latency')).toBeNull()
    expect(screen.queryByText('Error rate')).toBeNull()
    expect(screen.queryByText('Cache hit rate')).toBeNull()
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

  test('filters active channels by name or id', async () => {
    const user = userEvent.setup()
    renderPage()
    await waitFor(() => {
      expect(screen.getByText('Primary')).toBeInTheDocument()
    })

    const search = screen.getByRole('textbox', { name: 'Search channels' })
    await user.type(search, 'missing')
    expect(screen.queryByText('Primary')).toBeNull()
    expect(
      screen.getByText('No channels match your search')
    ).toBeInTheDocument()

    await user.clear(search)
    await user.type(search, '1')
    expect(screen.getByText('Primary')).toBeInTheDocument()
  })

  test('filters by exact group membership and restores all active channels when cleared', async () => {
    const user = userEvent.setup()
    renderPage({
      channels: [
        channel({ groups: ['default', 'vip'] }),
        channel({
          channel_id: 2,
          channel_name: 'VIP Plus',
          groups: ['vip-plus'],
        }),
        channel({ channel_id: 3, channel_name: 'Ungrouped', groups: [] }),
        channel({
          channel_id: 4,
          channel_name: 'Idle',
          groups: ['vip'],
          has_data: false,
        }),
      ],
    })
    await screen.findByText('Primary')

    const groupFilter = screen.getByRole('combobox', {
      name: 'Filter by group',
    })
    await user.click(groupFilter)
    await user.click(await screen.findByRole('option', { name: 'vip' }))

    expect(screen.getByText('Primary')).toBeInTheDocument()
    expect(screen.queryByText('VIP Plus')).not.toBeInTheDocument()
    expect(screen.queryByText('Ungrouped')).not.toBeInTheDocument()
    expect(screen.queryByText('Idle')).not.toBeInTheDocument()

    await user.click(groupFilter)
    await user.click(await screen.findByRole('option', { name: 'All Groups' }))
    expect(screen.getByText('VIP Plus')).toBeInTheDocument()
    expect(screen.getByText('Ungrouped')).toBeInTheDocument()
    expect(screen.queryByText('Idle')).not.toBeInTheDocument()
  })

  test('combines the group filter with search and preserves it when the time range changes', async () => {
    const user = userEvent.setup()
    const requestedRanges = renderPage({
      channels: [
        channel({ groups: ['vip'], cache_hit_rate: 50 }),
        channel({
          channel_id: 2,
          channel_name: 'Secondary',
          groups: ['default'],
        }),
      ],
    })
    await screen.findByText('Primary')
    const groupFilter = screen.getByRole('combobox', {
      name: 'Filter by group',
    })
    await user.click(groupFilter)
    await user.click(await screen.findByRole('option', { name: 'vip' }))

    const search = screen.getByRole('textbox', { name: 'Search channels' })
    await user.type(search, 'Secondary')
    expect(
      screen.getByText('No channels match your search')
    ).toBeInTheDocument()
    expect(screen.queryByText('Secondary')).not.toBeInTheDocument()

    await user.clear(search)
    await user.type(search, '1')
    expect(screen.getByText('Primary')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: '7d' }))
    await waitFor(() => expect(requestedRanges).toContain('7d'))
    expect(groupFilter).toHaveTextContent('vip')
    expect(screen.getByText('Primary')).toBeInTheDocument()
    expect(screen.queryByText('Secondary')).not.toBeInTheDocument()

    // The compact page summary stays fleet-wide, not request-group scoped.
    expect(screen.getByText('2 with traffic')).toBeInTheDocument()
    expect(screen.getAllByText('Consumption')).toHaveLength(3)
  })

  test('allows keyboard group selection and shows an empty state for a group without traffic', async () => {
    const user = userEvent.setup()
    renderPage({
      channels: [
        channel({ groups: ['default'] }),
        channel({
          channel_id: 2,
          channel_name: 'Idle',
          groups: ['idle'],
          has_data: false,
        }),
      ],
    })
    await screen.findByText('Primary')
    const groupFilter = screen.getByRole('combobox', {
      name: 'Filter by group',
    })
    groupFilter.focus()
    await user.keyboard('{ArrowDown}')
    expect(groupFilter).toHaveAttribute('aria-expanded', 'true')
    await screen.findByRole('option', { name: 'idle' })
    await user.keyboard('{End}{Enter}')

    expect(groupFilter).toHaveTextContent('idle')
    expect(groupFilter).toHaveAttribute('aria-expanded', 'false')
    expect(
      screen.getByText('No channel received traffic in this window')
    ).toBeInTheDocument()
    expect(screen.queryByText('Primary')).not.toBeInTheDocument()
    expect(screen.queryByText('Idle')).not.toBeInTheDocument()
  })

  test('keeps long group labels accessible without widening the responsive filters', async () => {
    const user = userEvent.setup()
    const group =
      'long-group-name-for-mobile-overflow-validation-abcdefghijklmnop'
    renderPage({ channels: [channel({ groups: [group] })] })
    await screen.findByText('Primary')

    const groupFilter = screen.getByRole('combobox', {
      name: 'Filter by group',
    })
    expect(groupFilter).toHaveClass('w-full', 'sm:w-48')
    expect(groupFilter.parentElement).toHaveClass('flex-col', 'sm:flex-row')
    await user.click(groupFilter)
    const option = await screen.findByRole('option', { name: group })
    expect(within(option).getByText(group)).toHaveClass(
      'wrap-anywhere',
      'whitespace-normal'
    )
    await user.click(option)

    expect(within(groupFilter).getByTitle(group)).toHaveTextContent(group)
    expect(within(groupFilter).getByText(group)).toHaveClass('truncate')
    expect(screen.getByText('Primary')).toBeInTheDocument()
  })

  test('uses a one two three-column channel card grid', async () => {
    renderPage()

    const channelList = await screen.findByRole('list', {
      name: 'Channel Availability',
    })
    expect(channelList).toHaveClass('grid', 'md:grid-cols-2', 'xl:grid-cols-3')
    expect(within(channelList).getAllByRole('listitem')).toHaveLength(1)
  })

  test('omits the idle toggle and monitoring caliber paragraphs', async () => {
    renderPage()
    await screen.findByText('Primary')

    expect(screen.queryByRole('switch')).toBeNull()
    expect(screen.queryByText(/Built from real traffic/)).toBeNull()
    expect(
      screen.queryByText(/Cache hit rate counts settled requests/)
    ).toBeNull()
  })
})

describe('ChannelMonitoring fleet totals board', () => {
  test('shows all three windows at once, with the short window selected by default', async () => {
    renderPage()
    await screen.findByText('Primary')

    // Today, this month and all time are all on screen simultaneously; the
    // quick selector only chooses which one is emphasised.
    expect(screen.getByRole('button', { name: 'Today' })).toHaveAttribute(
      'aria-pressed',
      'true'
    )
    expect(screen.getByRole('button', { name: 'This month' })).toHaveAttribute(
      'aria-pressed',
      'false'
    )
    expect(screen.getByRole('button', { name: 'All time' })).toHaveAttribute(
      'aria-pressed',
      'false'
    )
    expect(screen.getAllByText('Consumption')).toHaveLength(3)
    expect(screen.getAllByText('Cost')).toHaveLength(3)
    expect(screen.getAllByText('Profit')).toHaveLength(3)
  })

  test('moves the quick selection without hiding the other windows', async () => {
    renderPage()
    await screen.findByText('Primary')

    await userEvent.click(screen.getByRole('button', { name: 'All time' }))

    expect(screen.getByRole('button', { name: 'All time' })).toHaveAttribute(
      'aria-pressed',
      'true'
    )
    expect(screen.getByRole('button', { name: 'Today' })).toHaveAttribute(
      'aria-pressed',
      'false'
    )
    expect(screen.getAllByText('Consumption')).toHaveLength(3)
    expect(screen.getByText('Since the first request')).toBeInTheDocument()
  })

  test('marks a window whose cost was inferred from the current channel ratio', async () => {
    const inferred = totals({ cost_has_estimate: false })
    const withEstimate: ChannelMonitoringTotalsResponse = {
      available: true,
      totals: {
        today: { ...inferred.totals.today, cost_has_estimate: true },
        month: inferred.totals.month,
        total: inferred.totals.total,
      },
    }
    renderPage({ totals: withEstimate })
    await screen.findByText('Primary')

    // Only the window that actually carried an inferred ratio is flagged, so
    // an approximate figure is never presented as a settled one.
    expect(screen.getAllByText('estimated')).toHaveLength(1)
  })

  test('renders a dash rather than a zero when the totals are unavailable', async () => {
    renderPage({ totals: { available: false } })
    await screen.findByText('Primary')

    // Consume logging is off, so the figures are unknown. Showing 0 would
    // claim "no spend", which is a different statement.
    expect(screen.getAllByText('--')).toHaveLength(3)
    expect(screen.queryByText('Consumption')).toBeNull()
  })
})
