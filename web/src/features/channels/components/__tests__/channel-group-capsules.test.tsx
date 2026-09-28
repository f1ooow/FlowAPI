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
import { beforeEach, describe, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import { createGroupCapsuleClassName } from '../../lib'
import { channelSchema, type Channel } from '../../types'
import { ChannelGroupCapsules } from '../channel-group-capsules'
import { ChannelsProvider } from '../channels-provider'

const LONG_GROUP = '【职刻】Claude 逆向综合'
const GROUP_LIST = ['CODEX', 'GEM', LONG_GROUP]
// The colours the capsules must show once the group list has loaded.
const listedClassName = createGroupCapsuleClassName(GROUP_LIST)

function channel(group: string): Channel {
  return channelSchema.parse({
    id: 7,
    type: 1,
    key: 'sk-7',
    status: 1,
    name: 'FAST CODEX PRO',
    group,
    created_time: 0,
    test_time: 0,
    response_time: 0,
    balance_updated_time: 0,
  })
}

// The name cell puts the capsules in its wrapping name row; this harness
// mirrors that row so the test reads the layout the table renders.
function renderCapsules(target: Channel): HTMLElement {
  vi.spyOn(api, 'get').mockResolvedValue({
    data: { success: true, data: GROUP_LIST },
  })
  render(
    <QueryClientProvider client={new QueryClient()}>
      <ChannelsProvider>
        <div data-testid='name-row' className='flex flex-wrap items-center'>
          <span>{target.name}</span>
          <ChannelGroupCapsules channel={target} />
        </div>
      </ChannelsProvider>
    </QueryClientProvider>
  )
  return screen.getByTestId('name-row')
}

beforeEach(() => {
  localStorage.clear()
})

describe('ChannelGroupCapsules', () => {
  test('renders one solid capsule per group in the same row as the name', async () => {
    const row = renderCapsules(channel('CODEX,GEM'))

    const capsules = within(row).getAllByRole('button', {
      name: /^Edit groups: /,
    })
    expect(capsules).toHaveLength(2)
    // Direct children of the name row, not stacked in their own lines.
    for (const capsule of capsules) {
      expect(capsule.parentElement).toBe(row)
    }
    const codex = within(capsules[0]).getByText('CODEX')
    expect(codex).toHaveClass('text-white', 'rounded-md', 'text-xs')
    await waitFor(() => expect(codex).toHaveClass(listedClassName('CODEX')))
  })

  test('gives a group the same colour wherever it appears', async () => {
    vi.spyOn(api, 'get').mockResolvedValue({
      data: { success: true, data: GROUP_LIST },
    })
    // Two channels listing the group in different positions.
    render(
      <QueryClientProvider client={new QueryClient()}>
        <ChannelsProvider>
          <ChannelGroupCapsules channel={channel('GEM,CODEX')} />
          <ChannelGroupCapsules channel={channel('CODEX')} />
        </ChannelsProvider>
      </QueryClientProvider>
    )

    await waitFor(() =>
      expect(screen.getByText('GEM')).toHaveClass(listedClassName('GEM'))
    )
    const [first, second] = screen.getAllByText('CODEX')
    expect(first).toHaveClass(listedClassName('CODEX'))
    expect(first.className).toBe(second.className)
    expect(listedClassName('CODEX')).not.toBe(listedClassName('GEM'))
  })

  test('colours a group missing from the list by the hashed fallback', async () => {
    // RETIRED is still on the channel but no longer in the group list.
    renderCapsules(channel('CODEX,RETIRED'))

    await waitFor(() =>
      expect(screen.getByText('CODEX')).toHaveClass(listedClassName('CODEX'))
    )
    expect(screen.getByText('RETIRED')).toHaveClass(
      createGroupCapsuleClassName([])('RETIRED')
    )
  })

  test('shows an outlined add button instead when the channel has no group', () => {
    const row = renderCapsules(channel(''))

    const add = within(row).getByRole('button', { name: 'Add group' })
    expect(add.parentElement).toBe(row)
    expect(add).toHaveClass('border-dashed')
    expect(
      within(row).queryByRole('button', { name: /^Edit groups/ })
    ).not.toBeInTheDocument()
  })

  test('keeps every popover option on one line in a two-column grid', async () => {
    const user = userEvent.setup()
    renderCapsules(channel('CODEX'))

    await user.click(screen.getByRole('button', { name: 'Edit groups: CODEX' }))
    const option = await screen.findByRole('checkbox', { name: LONG_GROUP })

    const options = option.closest('[data-slot="group-options"]')
    expect(options).not.toBeNull()
    // Two equal columns, sized so a long name cannot widen its own column.
    expect(options).toHaveClass(
      'grid-cols-[repeat(2,minmax(0,1fr))]'
    )
    // A long group name is truncated with its full text on the title instead of
    // wrapping the option onto a second line, which would break the row rhythm.
    const capsule = within(options as HTMLElement).getByText(LONG_GROUP)
    expect(capsule).toHaveTextContent(LONG_GROUP)
    expect(capsule).toHaveClass('truncate', 'whitespace-nowrap')
    expect(capsule).toHaveAttribute('title', LONG_GROUP)
    // The option row keeps one fixed height, so the two columns stay aligned.
    expect(option.closest('div')).toHaveClass('h-8', 'items-center')
  })

  test('colours each popover option like its capsule', async () => {
    const user = userEvent.setup()
    renderCapsules(channel('CODEX'))

    await user.click(screen.getByRole('button', { name: 'Edit groups: CODEX' }))
    const option = await screen.findByRole('checkbox', { name: 'GEM' })
    const options = option.closest('[data-slot="group-options"]') as HTMLElement

    expect(within(options).getByText('GEM')).toHaveClass(
      listedClassName('GEM'),
      'text-white'
    )
  })

  test('opens the same popover from every capsule', async () => {
    const user = userEvent.setup()
    renderCapsules(channel('CODEX,GEM'))

    await user.click(screen.getByRole('button', { name: 'Edit groups: GEM' }))

    expect(
      await screen.findByRole('textbox', { name: 'Search groups...' })
    ).toBeInTheDocument()
    expect(screen.getByRole('checkbox', { name: 'CODEX' })).toBeChecked()
    expect(screen.getByRole('checkbox', { name: 'GEM' })).toBeChecked()
    expect(screen.getByRole('checkbox', { name: LONG_GROUP })).not.toBeChecked()
  })
})
