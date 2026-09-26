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
import { afterEach, describe, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { aggregateChannelsByTag, isTagAggregateRow } from '../../lib'
import { channelSchema, type Channel } from '../../types'
import { ChannelRoutingCell } from '../channel-routing-cell'

function channel(id: number, costRatio: number): Channel {
  return channelSchema.parse({
    id,
    type: 1,
    key: `sk-${id}`,
    status: 1,
    name: `channel-${id}`,
    tag: 'pool',
    priority: 1,
    weight: 2,
    cost_ratio: costRatio,
    created_time: 0,
    test_time: 0,
    response_time: 0,
    balance_updated_time: 0,
  })
}

function renderCell(target: Channel): void {
  render(
    <QueryClientProvider client={new QueryClient()}>
      <ChannelRoutingCell channel={target} />
    </QueryClientProvider>
  )
}

function signIn(role: number): void {
  useAuthStore.getState().auth.setUser({ id: 1, username: 'admin', role })
}

afterEach(() => useAuthStore.getState().auth.reset())

describe('ChannelRoutingCell', () => {
  test('shows priority, weight and cost ratio as chips', () => {
    signIn(ROLE.ADMIN)
    renderCell(channel(1, 0.2))

    expect(screen.getByRole('button', { name: 'Priority 1' })).toBeVisible()
    expect(screen.getByRole('button', { name: 'Weight 2' })).toBeVisible()
    expect(screen.getByText('0.2x')).toBeVisible()
  })

  test('keeps the cost ratio read-only for an admin below super admin', () => {
    signIn(ROLE.ADMIN)
    renderCell(channel(1, 0.2))

    expect(
      screen.queryByRole('button', { name: 'Cost ratio 0.2x' })
    ).not.toBeInTheDocument()
  })

  test('lets a super admin save a new cost ratio', async () => {
    signIn(ROLE.SUPER_ADMIN)
    const user = userEvent.setup()
    const put = vi
      .spyOn(api, 'put')
      .mockResolvedValue({ data: { success: true } })
    renderCell(channel(1, 0.2))

    await user.click(screen.getByRole('button', { name: 'Cost ratio 0.2x' }))
    const input = screen.getByRole('textbox', { name: 'Cost ratio' })
    await user.clear(input)
    await user.type(input, '0.25{Enter}')

    expect(put).toHaveBeenCalledWith(
      '/api/channel/',
      { id: 1, cost_ratio: 0.25 },
      expect.anything()
    )
  })

  test('asks before changing the weight of every channel in a tag', async () => {
    signIn(ROLE.ADMIN)
    const user = userEvent.setup()
    const put = vi
      .spyOn(api, 'put')
      .mockResolvedValue({ data: { success: true } })
    const row = aggregateChannelsByTag([channel(1, 0.2), channel(2, 0.5)])[0]
    if (!row || !isTagAggregateRow(row)) {
      throw new Error('Missing tag row')
    }
    renderCell(row)
    expect(screen.getByText('0.2x-0.5x')).toBeVisible()

    await user.click(screen.getByRole('button', { name: 'Weight 2' }))
    const input = screen.getByRole('textbox', { name: 'Weight' })
    await user.clear(input)
    await user.type(input, '5{Enter}')
    expect(put).not.toHaveBeenCalled()

    await user.click(await screen.findByRole('button', { name: 'Update' }))

    await waitFor(() =>
      expect(put).toHaveBeenCalledWith(
        '/api/channel/tag',
        { tag: 'pool', weight: 5 },
        expect.anything()
      )
    )
  })
})
