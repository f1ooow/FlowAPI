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
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ReactNode } from 'react'
import { describe, expect, test, vi } from 'vitest'

import type { ChannelMonitoringChannelSummary } from '@/features/channel-monitoring/types'
import { api } from '@/lib/api'

import { channelSchema } from '../../types'
import { ChannelHealthCell } from '../channel-health-cell'
import { ChannelHealthDrawer } from '../channel-health-drawer'
import { ChannelHealthOverview } from '../channel-health-overview'
import { ChannelHealthProvider } from '../channel-health-provider'

const channel = channelSchema.parse({
  id: 12,
  type: 1,
  key: '',
  status: 1,
  name: 'Primary relay',
  created_time: 0,
  test_time: 0,
  response_time: 0,
  balance_updated_time: 0,
})

const health: ChannelMonitoringChannelSummary = {
  channel_id: 12,
  channel_name: 'Primary relay',
  groups: ['default'],
  today_used_quota: null,
  today_cost_quota: null,
  has_data: true,
  state: 'healthy',
  availability_rate: 99.5,
  error_rate: 0.5,
  avg_latency_ms: 1840,
  attempt_count: 200,
  success_count: 199,
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
    { ts: 1800, attempt_count: 200, success_count: 199, state: 'healthy' },
  ],
}

function renderHealth(children: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={client}>
      <ChannelHealthProvider>{children}</ChannelHealthProvider>
    </QueryClientProvider>
  )
}

describe('Channel health', () => {
  test('multiple channel cells share one summary request and display real attempt metrics', async () => {
    const get = vi.spyOn(api, 'get').mockResolvedValue({
      data: {
        success: true,
        data: { range: '24h', ranges: ['1h', '24h'], channels: [health] },
      },
    })
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    render(
      <QueryClientProvider client={client}>
        <ChannelHealthProvider>
          <ChannelHealthCell channel={channel} />
          <ChannelHealthCell
            channel={{ ...channel, id: 13, name: 'Idle relay' }}
          />
        </ChannelHealthProvider>
      </QueryClientProvider>
    )

    const button = await screen.findByRole('button', {
      name: 'View channel health for Primary relay',
    })
    expect(within(button).getByText('99.50%')).toBeInTheDocument()
    expect(within(button).getByText('200')).toBeInTheDocument()
    expect(within(button).getByText('1840 ms')).toBeInTheDocument()
    expect(within(button).getByRole('img').childElementCount).toBe(2)
    expect(get).toHaveBeenCalledTimes(1)
    expect(get).toHaveBeenCalledWith('/api/channel-monitoring/summary', {
      params: { range: '24h' },
    })
  })

  test('a channel without traffic shows no-data placeholders rather than zero availability', async () => {
    vi.spyOn(api, 'get').mockResolvedValue({
      data: {
        success: true,
        data: {
          range: '24h',
          ranges: ['24h'],
          channels: [
            { ...health, has_data: false, state: 'no-data', attempt_count: 0 },
          ],
        },
      },
    })
    renderHealth(<ChannelHealthCell channel={channel} />)

    const button = await screen.findByRole('button', {
      name: 'View channel health for Primary relay',
    })
    expect(within(button).getByText('No data')).toBeInTheDocument()
    expect(within(button).getAllByText('--')).toHaveLength(2)
    expect(within(button).queryByText('0.00%')).not.toBeInTheDocument()
  })

  test('a summary request failure is visible and is not reported as no traffic', async () => {
    vi.spyOn(api, 'get').mockRejectedValue(new Error('unavailable'))
    renderHealth(<ChannelHealthCell channel={channel} />)

    expect(
      await screen.findByText('Health data unavailable')
    ).toBeInTheDocument()
    expect(screen.queryByText('No data')).not.toBeInTheDocument()
    expect(screen.queryByRole('img')).not.toBeInTheDocument()
  })

  test('clicking a channel health cell opens its detailed health drawer and Escape closes it', async () => {
    vi.spyOn(api, 'get').mockResolvedValue({
      data: {
        success: true,
        data: { range: '24h', ranges: ['1h', '24h'], channels: [health] },
      },
    })
    const user = userEvent.setup()
    renderHealth(
      <>
        <ChannelHealthCell channel={channel} />
        <ChannelHealthDrawer />
      </>
    )

    await user.click(
      await screen.findByRole('button', {
        name: 'View channel health for Primary relay',
      })
    )
    const dialog = await screen.findByRole('dialog', { name: 'Primary relay' })
    expect(within(dialog).getByText('Availability')).toBeInTheDocument()
    expect(within(dialog).getByText('Cache hit')).toBeInTheDocument()
    expect(
      within(dialog).getByRole('group', { name: 'Time range' })
    ).toBeInTheDocument()
    await user.keyboard('{Escape}')
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  })

  test('a tag aggregate prompts expansion instead of displaying partial channel health', async () => {
    vi.spyOn(api, 'get').mockResolvedValue({
      data: {
        success: true,
        data: { range: '24h', ranges: ['24h'], channels: [health] },
      },
    })
    const expand = vi.fn()
    const user = userEvent.setup()
    renderHealth(
      <ChannelHealthCell
        channel={{ ...channel, tag: 'Shared', children: [channel] }}
        onExpand={expand}
      />
    )

    await user.click(
      screen.getByRole('button', { name: 'Expand to view channel health' })
    )
    expect(expand).toHaveBeenCalledTimes(1)
    expect(screen.queryByText('99.50%')).not.toBeInTheDocument()
    expect(screen.queryByRole('img')).not.toBeInTheDocument()
  })

  test('the history drawer preserves fleet totals and deleted-channel metrics without per-channel requests', async () => {
    const totalsWindow = {
      used_quota: 1_000_000,
      cost_quota: 500_000,
      profit: 500_000,
      cost_has_estimate: false,
    }
    const get = vi.spyOn(api, 'get').mockImplementation(async (url) => {
      if (url === '/api/channel-monitoring/totals') {
        return {
          data: {
            success: true,
            data: {
              available: true,
              totals: {
                today: totalsWindow,
                month: totalsWindow,
                total: totalsWindow,
              },
            },
          },
        }
      }
      return {
        data: {
          success: true,
          data: {
            range: '24h',
            ranges: ['24h'],
            overall: health,
            updated_at: 1_800_000_000,
            channels: [
              health,
              { ...health, channel_id: 7, channel_name: '', groups: [] },
            ],
          },
        },
      }
    })
    const user = userEvent.setup()
    renderHealth(<ChannelHealthOverview />)

    expect(await screen.findByText('99.50%')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'History and totals' }))
    const dialog = await screen.findByRole('dialog', {
      name: 'History and totals',
    })
    expect(
      await within(dialog).findByRole('button', { name: 'All time' })
    ).toBeInTheDocument()
    expect(within(dialog).getByText('Deleted channel')).toBeInTheDocument()
    expect(within(dialog).getByText('#7')).toBeInTheDocument()
    expect(
      get.mock.calls.filter(
        ([url]) => url === '/api/channel-monitoring/summary'
      )
    ).toHaveLength(1)
    expect(
      get.mock.calls.filter(([url]) => url === '/api/channel-monitoring/totals')
    ).toHaveLength(1)
  })
})
