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
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, test } from 'vitest'

import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import type { Channel } from '../../types'
import { ChannelCostRatioCell } from '../channel-cost-ratio-cell'

const channel = {
  id: 42,
  cost_ratio: 0.75,
} as Channel

function renderCell() {
  const queryClient = new QueryClient()
  return render(
    <QueryClientProvider client={queryClient}>
      <ChannelCostRatioCell channel={channel} />
    </QueryClientProvider>
  )
}

describe('ChannelCostRatioCell', () => {
  afterEach(() => useAuthStore.getState().auth.reset())

  test('lets a super admin edit a channel cost ratio inline', async () => {
    useAuthStore.getState().auth.setUser({
      id: 1,
      username: 'root',
      role: ROLE.SUPER_ADMIN,
    })
    const user = userEvent.setup()
    renderCell()

    await user.click(screen.getByRole('button', { name: '0.75' }))

    expect(screen.getByRole('textbox')).toHaveValue('0.75')
  })

  test('keeps the ratio read-only for users without sensitive edit permission', () => {
    useAuthStore.getState().auth.setUser({
      id: 2,
      username: 'admin',
      role: ROLE.ADMIN,
    })
    renderCell()

    expect(
      screen.queryByRole('button', { name: '0.75' })
    ).not.toBeInTheDocument()
    expect(screen.getByText('0.75x')).toBeInTheDocument()
  })
})
