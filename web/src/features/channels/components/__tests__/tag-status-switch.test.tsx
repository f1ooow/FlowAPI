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

import {
  aggregateChannelsByTag,
  isTagAggregateRow,
  type TagRow,
} from '../../lib'
import { channelSchema } from '../../types'
import { TagStatusSwitch } from '../channel-status-switch'

function tagRow(statuses: number[]): TagRow {
  const children = statuses.map((status, index) =>
    channelSchema.parse({
      id: index + 1,
      type: 1,
      key: `sk-${index + 1}`,
      status,
      name: `channel-${index + 1}`,
      tag: 'pool',
      created_time: 0,
      test_time: 0,
      response_time: 0,
      balance_updated_time: 0,
    })
  )
  const row = aggregateChannelsByTag(children)[0]
  if (!row || !isTagAggregateRow(row)) {
    throw new Error('Missing tag row')
  }
  return row
}

function renderSwitch(row: TagRow): void {
  render(
    <QueryClientProvider client={new QueryClient()}>
      <TagStatusSwitch row={row} />
    </QueryClientProvider>
  )
}

describe('TagStatusSwitch', () => {
  test('reads on only when every channel of the tag is enabled', () => {
    renderSwitch(tagRow([1, 1]))

    expect(screen.getByRole('switch', { name: 'Enable All' })).toBeChecked()
    expect(screen.getByText('2/2')).toBeInTheDocument()
  })

  test('shows a partly enabled tag as off with the enabled count', () => {
    renderSwitch(tagRow([1, 2, 3]))

    expect(screen.getByRole('switch', { name: 'Enable All' })).not.toBeChecked()
    expect(screen.getByText('1/3')).toBeInTheDocument()
  })

  test('switching a partly enabled tag on enables every channel of it', async () => {
    const user = userEvent.setup()
    const post = vi
      .spyOn(api, 'post')
      .mockResolvedValue({ data: { success: true } })
    renderSwitch(tagRow([1, 2]))

    await user.click(screen.getByRole('switch', { name: 'Enable All' }))

    await waitFor(() => expect(post).toHaveBeenCalledTimes(1))
    expect(post).toHaveBeenCalledWith(
      '/api/channel/tag/enabled',
      { tag: 'pool' },
      expect.anything()
    )
  })

  test('switching a fully enabled tag off disables every channel of it', async () => {
    const user = userEvent.setup()
    const post = vi
      .spyOn(api, 'post')
      .mockResolvedValue({ data: { success: true } })
    renderSwitch(tagRow([1, 1]))

    await user.click(screen.getByRole('switch', { name: 'Enable All' }))

    await waitFor(() => expect(post).toHaveBeenCalledTimes(1))
    expect(post).toHaveBeenCalledWith(
      '/api/channel/tag/disabled',
      { tag: 'pool' },
      expect.anything()
    )
  })
})
