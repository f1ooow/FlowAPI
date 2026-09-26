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
import { afterEach, beforeEach, describe, expect, test } from 'vitest'

import { channelSchema, type Channel } from '../../types'
import { ChannelCard } from '../channel-card'
import { ChannelQuotaSummaryCell } from '../channel-quota-summary-cell'
import { ChannelsProvider, useChannels } from '../channels-provider'

type QuotaFields = Partial<
  Pick<
    Channel,
    | 'today_cost_quota'
    | 'today_used_quota'
    | 'total_cost_quota'
    | 'total_used_quota'
  >
>

// 500_000 quota is $1 under the default currency settings.
function channel(quotas: QuotaFields): Channel {
  return channelSchema.parse({
    id: 1,
    type: 1,
    key: 'sk-test',
    status: 1,
    name: 'Primary',
    created_time: 0,
    test_time: 0,
    response_time: 0,
    balance_updated_time: 0,
    ...quotas,
  })
}

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

function renderCell(target: Channel, scope: 'today' | 'total' = 'today'): void {
  render(
    <QueryClientProvider client={new QueryClient()}>
      <ChannelsProvider>
        <VisibilityToggle />
        <ChannelQuotaSummaryCell channel={target} scope={scope} />
      </ChannelsProvider>
    </QueryClientProvider>
  )
}

function renderCard(target: Channel): void {
  const row = {
    original: target,
    getAllCells: () => [
      {
        column: {
          id: 'quota_summary',
          columnDef: {
            cell: <ChannelQuotaSummaryCell channel={target} scope='today' />,
          },
        },
        getContext: () => ({}),
      },
    ],
  } as unknown as Row<Channel>

  render(
    <QueryClientProvider client={new QueryClient()}>
      <ChannelsProvider>
        <ChannelCard row={row} isSelected={false} />
      </ChannelsProvider>
    </QueryClientProvider>
  )
}

beforeEach(() => {
  localStorage.clear()
})

afterEach(() => {
  localStorage.clear()
})

describe('ChannelQuotaSummaryCell', () => {
  test('shows today cost, consumption and their difference as profit', () => {
    renderCell(
      channel({ today_cost_quota: 500_000, today_used_quota: 1_250_000 })
    )

    expect(screen.getByTitle('Cost: $1')).toHaveTextContent(/^\$1\.00$/)
    expect(screen.getByTitle('Consumption: $2.5')).toHaveTextContent(
      /^\$2\.50$/
    )
    expect(screen.getByTitle('Profit: $1.5')).toHaveTextContent(/^\$1\.50$/)
    expect(screen.getByTitle('Profit: $1.5')).toHaveClass('text-success')
  })

  test('reads the all-time fields when the scope is total', () => {
    renderCell(
      channel({
        today_cost_quota: 500_000,
        today_used_quota: 1_250_000,
        total_cost_quota: 5_000_000,
        total_used_quota: 7_500_000,
      }),
      'total'
    )

    expect(screen.getByTitle('Cost: $10')).toHaveTextContent(/^\$10\.00$/)
    expect(screen.getByTitle('Consumption: $15')).toHaveTextContent(
      /^\$15\.00$/
    )
    expect(screen.getByTitle('Profit: $5')).toHaveTextContent(/^\$5\.00$/)
    expect(screen.queryByText('$2.50')).not.toBeInTheDocument()
  })

  test('marks a loss in the danger colour', () => {
    renderCell(
      channel({ today_cost_quota: 2_000_000, today_used_quota: 1_250_000 })
    )

    expect(screen.getByTitle('Profit: -$1.5')).toHaveTextContent(/^-\$1\.50$/)
    expect(screen.getByTitle('Profit: -$1.5')).toHaveClass('text-destructive')
  })

  test('keeps zero as a valid amount in the neutral colour', () => {
    renderCell(channel({ today_cost_quota: 0, today_used_quota: 0 }))

    expect(screen.getAllByText('$0.00')).toHaveLength(3)
    expect(screen.queryByText('--')).not.toBeInTheDocument()
    expect(screen.getByTitle('Profit: $0')).not.toHaveClass('text-success')
    expect(screen.getByTitle('Profit: $0')).not.toHaveClass('text-destructive')
  })

  test('rounds every amount to two decimals, keeping the precise value in the title', () => {
    // The production report: $0.8282 consumed against a $0.0001 cost.
    renderCell(channel({ today_cost_quota: 50, today_used_quota: 414_100 }))

    expect(screen.getByTitle('Cost: $0.0001')).toHaveTextContent(/^\$0\.00$/)
    expect(screen.getByTitle('Consumption: $0.8282')).toHaveTextContent(
      /^\$0\.83$/
    )
    expect(screen.getByTitle('Profit: $0.8281')).toHaveTextContent(/^\$0\.83$/)
  })

  test('colours a profit that rounds to 0.00 by its real sign', () => {
    // A $0.0001 loss: shown as zero, still marked as a loss.
    renderCell(
      channel({ today_cost_quota: 414_150, today_used_quota: 414_100 })
    )

    expect(screen.getByTitle('Profit: -$0.0001')).toHaveTextContent(/^\$0\.00$/)
    expect(screen.getByTitle('Profit: -$0.0001')).toHaveClass(
      'text-destructive'
    )
  })

  test.each([
    { name: 'null', cost: null },
    { name: 'missing from the response', cost: undefined },
  ])(
    'shows dashes for the cost and the profit when the cost is $name',
    ({ cost }) => {
      const target = channel({ today_used_quota: 1_250_000 })
      renderCell({ ...target, today_cost_quota: cost as number | null })

      expect(screen.getByTitle('Cost: --')).toHaveTextContent('--')
      expect(screen.getByTitle('Consumption: $2.5')).toBeInTheDocument()
      expect(screen.getByTitle('Profit: --')).toHaveTextContent('--')
    }
  )

  test('shows dashes for consumption and profit when consumption is unavailable', () => {
    renderCell(channel({ today_cost_quota: 500_000, today_used_quota: null }))

    expect(screen.getByTitle('Cost: $1')).toBeInTheDocument()
    expect(screen.getByTitle('Consumption: --')).toBeInTheDocument()
    expect(screen.getByTitle('Profit: --')).toBeInTheDocument()
  })

  test('shows large amounts in full with thousands separators', () => {
    renderCell(
      channel({ today_cost_quota: 0, today_used_quota: 500_000_000_000 })
    )

    expect(screen.getByTitle('Consumption: $1,000,000')).toHaveTextContent(
      /^\$1,000,000\.00$/
    )
  })

  test('masks every amount and the profit colour when values are hidden', async () => {
    const user = userEvent.setup()
    renderCell(
      channel({ today_cost_quota: 2_000_000, today_used_quota: 1_250_000 })
    )

    await user.click(screen.getByRole('button', { name: 'Hide values' }))

    expect(screen.getAllByText('••••')).toHaveLength(3)
    expect(screen.queryByText('-$1.50')).not.toBeInTheDocument()
    expect(screen.getByTitle('Profit: ••••')).not.toHaveClass(
      'text-destructive'
    )
  })

  test('keeps unavailable values as dashes when values are hidden', async () => {
    const user = userEvent.setup()
    renderCell(channel({ today_cost_quota: null, today_used_quota: 1_250_000 }))

    await user.click(screen.getByRole('button', { name: 'Hide values' }))

    expect(screen.getByTitle('Cost: --')).toBeInTheDocument()
    expect(screen.getByTitle('Consumption: ••••')).toBeInTheDocument()
    expect(screen.getByTitle('Profit: --')).toBeInTheDocument()
  })

  test('labels the card field with the scope and drops the currency symbol', () => {
    renderCard(
      channel({ today_cost_quota: 500_000, today_used_quota: 1_250_000 })
    )

    expect(
      screen.getByText('Cost / Consumption / Profit · Today')
    ).toBeInTheDocument()
    expect(screen.getByTitle('Cost: 1')).toHaveTextContent(/^1\.00$/)
    expect(screen.getByTitle('Consumption: 2.5')).toHaveTextContent(/^2\.50$/)
    expect(screen.getByTitle('Profit: 1.5')).toHaveTextContent(/^1\.50$/)
  })
})
