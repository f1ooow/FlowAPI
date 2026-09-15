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
import type { Row } from '@tanstack/react-table'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'

import type { Channel } from '../../types'
import { ChannelCard } from '../channel-card'
import { ChannelTodayConsumptionCell } from '../channel-today-consumption-cell'
import { ChannelsProvider, useChannels } from '../channels-provider'

function VisibilityToggle() {
  const { sensitiveVisible, setSensitiveVisible } = useChannels()
  return (
    <button
      type='button'
      onClick={() => setSensitiveVisible(!sensitiveVisible)}
    >
      Hide values
    </button>
  )
}

function renderCell(todayUsedQuota: number | null) {
  const queryClient = new QueryClient()
  return render(
    <QueryClientProvider client={queryClient}>
      <ChannelsProvider>
        <VisibilityToggle />
        <ChannelTodayConsumptionCell
          channel={{ id: 1, today_used_quota: todayUsedQuota } as Channel}
        />
      </ChannelsProvider>
    </QueryClientProvider>
  )
}

function renderCard(todayUsedQuota: number) {
  const queryClient = new QueryClient()
  const channel = {
    id: 1,
    status: 1,
    group: '',
    today_used_quota: todayUsedQuota,
  } as Channel
  const row = {
    original: channel,
    getAllCells: () => [
      {
        column: {
          id: 'today_used_quota',
          columnDef: {
            cell: <ChannelTodayConsumptionCell channel={channel} />,
          },
        },
        getContext: () => ({}),
      },
    ],
  } as unknown as Row<Channel>

  return render(
    <QueryClientProvider client={queryClient}>
      <ChannelsProvider>
        <ChannelCard row={row} isSelected={false} />
      </ChannelsProvider>
    </QueryClientProvider>
  )
}

beforeEach(() => {
  const storage = new Map<string, string>()
  vi.stubGlobal('localStorage', {
    getItem: (key: string) => storage.get(key) ?? null,
    setItem: (key: string, value: string) => storage.set(key, value),
    removeItem: (key: string) => storage.delete(key),
  })
})

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('ChannelTodayConsumptionCell', () => {
  test('shows zero as a valid formatted amount', () => {
    renderCell(0)

    expect(screen.getByText('$0')).toBeInTheDocument()
    expect(screen.queryByText('--')).not.toBeInTheDocument()
  })

  test('shows unavailable usage as dashes even when values are hidden', async () => {
    const user = userEvent.setup()
    renderCell(null)

    expect(screen.getByText('--')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Hide values' }))
    expect(screen.getByText('--')).toBeInTheDocument()
    expect(screen.queryByText('••••')).not.toBeInTheDocument()
  })

  test('keeps the precise value in the tooltip and masks all numeric output', async () => {
    const user = userEvent.setup()
    renderCell(500_000_000_000)

    expect(screen.getByText('$1M')).toBeInTheDocument()
    expect(screen.getByTitle('$1,000,000')).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Hide values' }))
    expect(screen.getByText('••••')).toBeInTheDocument()
    expect(screen.queryByTitle('$1,000,000')).not.toBeInTheDocument()
  })

  test('includes the daily consumption field in the mobile card', () => {
    renderCard(5_000_000)

    expect(screen.getByText('Consumed today')).toBeInTheDocument()
    expect(screen.getByTitle('10')).toBeInTheDocument()
  })
})
