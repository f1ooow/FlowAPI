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
import { beforeEach, describe, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import { channelSchema, type Channel } from '../../types'
import { ChannelGroupsCell } from '../channel-groups-cell'
import { ChannelsProvider, useChannels } from '../channels-provider'

const LONG_GROUP = '【职刻】Claude 逆向综合'
const GROUP_LIST = ['CODEX', 'GEM', LONG_GROUP]

function channel(id: number, group: string): Channel {
  return channelSchema.parse({
    id,
    type: 1,
    key: `sk-${id}`,
    status: 1,
    name: `channel-${id}`,
    group,
    created_time: 0,
    test_time: 0,
    response_time: 0,
    balance_updated_time: 0,
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

function renderCells(...targets: Channel[]): void {
  vi.spyOn(api, 'get').mockResolvedValue({
    data: { success: true, data: GROUP_LIST },
  })
  render(
    <QueryClientProvider client={new QueryClient()}>
      <ChannelsProvider>
        <VisibilityToggle />
        {targets.map((target) => (
          <div key={target.id} data-testid={`cell-${target.id}`}>
            <ChannelGroupsCell channel={target} />
          </div>
        ))}
      </ChannelsProvider>
    </QueryClientProvider>
  )
}

function triggerOf(id: number): HTMLElement {
  return within(screen.getByTestId(`cell-${id}`)).getByRole('button')
}

beforeEach(() => {
  localStorage.clear()
})

describe('ChannelGroupsCell summary', () => {
  test('names the first two groups and folds the rest into a count', () => {
    renderCells(channel(1, 'GEM,default,team-a,CODEX,image'))

    const trigger = triggerOf(1)
    // default leads, the rest follow by name.
    expect(within(trigger).getByText('default, CODEX')).toBeVisible()
    expect(within(trigger).getByText('+3')).toBeVisible()
    // The full list stays one hover away and is the trigger's accessible name.
    expect(trigger).toHaveAttribute(
      'title',
      'default, CODEX, GEM, image, team-a'
    )
    expect(trigger).toHaveAccessibleName(
      'Edit groups: default, CODEX, GEM, image, team-a'
    )
  })

  test('shows every group without a count when there are two or fewer', () => {
    renderCells(channel(1, 'CODEX,GEM'), channel(2, 'GEM'))

    expect(triggerOf(1)).toHaveTextContent(/^CODEX, GEM$/)
    expect(triggerOf(2)).toHaveTextContent(/^GEM$/)
  })

  test('sizes the summary the same for one group and for a hundred', () => {
    const hundred = Array.from(
      { length: 100 },
      (_, i) => `g${String(i + 1).padStart(3, '0')}`
    )
    renderCells(channel(1, 'GEM'), channel(2, hundred.join(',')))

    const single = triggerOf(1)
    const many = triggerOf(2)
    expect(within(many).getByText('+98')).toBeVisible()
    // A constant intrinsic width that fills the column, on one line: the
    // column cannot grow with the groups, and the row cannot wrap.
    expect(single).toHaveClass('w-24', 'min-w-full', 'whitespace-nowrap')
    expect(many.className).toBe(single.className)
    // The names give way inside that width; the count never does.
    expect(within(many).getByText('g001, g002')).toHaveClass(
      'min-w-0',
      'truncate'
    )
    expect(within(many).getByText('+98')).toHaveClass('shrink-0')
  })

  test('lists the groups as quiet text without colour fills', () => {
    renderCells(channel(1, 'CODEX,GEM'))

    const trigger = triggerOf(1)
    const names = within(trigger).getByText('CODEX, GEM')
    expect(trigger).toHaveClass('text-muted-foreground')
    expect(names.className).not.toMatch(/\bbg-/)
    expect(names).not.toHaveClass('text-white')
  })

  test('masks the names but keeps the count when values are hidden', async () => {
    const user = userEvent.setup()
    renderCells(channel(1, 'CODEX,GEM,image'))

    await user.click(screen.getByRole('button', { name: 'Hide values' }))

    const trigger = triggerOf(1)
    expect(within(trigger).getByText('••••, ••••')).toBeVisible()
    expect(within(trigger).getByText('+1')).toBeVisible()
    expect(trigger).not.toHaveTextContent('CODEX')
    expect(trigger).toHaveAttribute('title', '••••, ••••, ••••')
  })

  test('shows an outlined add button instead when the channel has no group', () => {
    renderCells(channel(1, ''))

    const add = screen.getByRole('button', { name: 'Add group' })
    expect(add).toHaveClass('border-dashed')
    expect(
      screen.queryByRole('button', { name: /^Edit groups/ })
    ).not.toBeInTheDocument()
  })
})

describe('ChannelGroupsCell editor', () => {
  test('opens a grid with the channel groups ticked', async () => {
    const user = userEvent.setup()
    renderCells(channel(1, 'CODEX,GEM'))

    await user.click(triggerOf(1))

    expect(
      await screen.findByRole('textbox', { name: 'Search groups...' })
    ).toBeInTheDocument()
    expect(screen.getByRole('checkbox', { name: 'CODEX' })).toBeChecked()
    expect(screen.getByRole('checkbox', { name: 'GEM' })).toBeChecked()
    expect(screen.getByRole('checkbox', { name: LONG_GROUP })).not.toBeChecked()
  })

  test('keeps every option on one line in a two-column grid', async () => {
    const user = userEvent.setup()
    renderCells(channel(1, 'CODEX'))

    await user.click(triggerOf(1))
    const option = await screen.findByRole('checkbox', { name: LONG_GROUP })

    const options = option.closest('[data-slot="group-options"]')
    expect(options).not.toBeNull()
    // Two equal columns, sized so a long name cannot widen its own column.
    expect(options).toHaveClass('grid-cols-[repeat(2,minmax(0,1fr))]')
    // A long group name is truncated with its full text on the title instead of
    // wrapping the option onto a second line, which would break the row rhythm.
    const label = within(options as HTMLElement).getByText(LONG_GROUP)
    expect(label).toHaveClass('truncate', 'whitespace-nowrap')
    expect(label).toHaveAttribute('title', LONG_GROUP)
    // The option row keeps one fixed height, so the two columns stay aligned.
    expect(option.closest('div')).toHaveClass('h-8', 'items-center')
  })

  test('distinguishes a ticked option by weight and contrast, not colour', async () => {
    const user = userEvent.setup()
    // CODEX is on the channel, GEM is not.
    renderCells(channel(1, 'CODEX'))

    await user.click(triggerOf(1))
    const options = (
      await screen.findByRole('checkbox', { name: 'GEM' })
    ).closest('[data-slot="group-options"]') as HTMLElement

    const ticked = within(options).getByText('CODEX')
    const unticked = within(options).getByText('GEM')
    expect(ticked).toHaveClass('text-foreground', 'font-medium')
    expect(unticked).toHaveClass('text-muted-foreground')
    expect(ticked.className).not.toMatch(/\bbg-/)
  })
})
