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
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import { createGroupCapsuleClassName } from '../../lib'
import { channelSchema, type Channel } from '../../types'
import { ChannelsProvider } from '../channels-provider'
import { ChannelsTable } from '../channels-table'

// The brand icon package uses directory imports unsupported by Node's ESM loader.
vi.mock('@lobehub/icons', () => new Proxy({}, { has: () => true }))

type SearchRecord = Record<string, unknown>

// Stands in for the router's validated search params: navigate() rewrites the
// URL state and every reader re-renders with it.
const urlSearch = vi.hoisted(() => {
  let search: SearchRecord = {}
  const listeners = new Set<() => void>()
  return {
    get: (): SearchRecord => search,
    set(next: SearchRecord) {
      search = Object.fromEntries(
        Object.entries(next).filter(([, value]) => value !== undefined)
      )
      for (const listener of listeners) {
        listener()
      }
    },
    subscribe(listener: () => void) {
      listeners.add(listener)
      return () => {
        listeners.delete(listener)
      }
    },
  }
})

vi.mock('@tanstack/react-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@tanstack/react-router')>()
  const { useSyncExternalStore } = await import('react')
  const navigate = (options: {
    search: SearchRecord | ((previous: SearchRecord) => SearchRecord)
  }) => {
    urlSearch.set(
      typeof options.search === 'function'
        ? options.search(urlSearch.get())
        : options.search
    )
  }
  return {
    ...actual,
    getRouteApi: () => ({
      useSearch: () => useSyncExternalStore(urlSearch.subscribe, urlSearch.get),
      useNavigate: () => navigate,
    }),
  }
})

// 500_000 quota is $1 under the default currency settings.
const initialChannels: Channel[] = [
  channelSchema.parse({
    id: 1,
    type: 1,
    key: 'sk-1',
    status: 1,
    name: 'OpenAI main',
    group: 'default',
    created_time: 0,
    test_time: 0,
    response_time: 0,
    balance_updated_time: 0,
    today_cost_quota: 500_000,
    today_used_quota: 1_250_000,
    total_cost_quota: 5_000_000,
    total_used_quota: 7_500_000,
  }),
  channelSchema.parse({
    id: 2,
    type: 14,
    key: 'sk-2',
    status: 1,
    name: 'Claude vip',
    group: 'vip',
    created_time: 0,
    test_time: 0,
    response_time: 0,
    balance_updated_time: 0,
  }),
  channelSchema.parse({
    id: 3,
    type: 999,
    key: 'sk-3',
    status: 1,
    name: 'Legacy relay',
    group: 'default',
    created_time: 0,
    test_time: 0,
    response_time: 0,
    balance_updated_time: 0,
  }),
  channelSchema.parse({
    id: 4,
    type: 1,
    key: 'sk-4a\nsk-4b\nsk-4c',
    status: 3,
    name: 'Flaky pool',
    group: 'default',
    created_time: 0,
    test_time: 0,
    response_time: 0,
    balance_updated_time: 0,
    other_info: JSON.stringify({ status_reason: 'quota exhausted' }),
    channel_info: {
      is_multi_key: true,
      multi_key_size: 3,
      multi_key_status_list: { '1': 3 },
      multi_key_polling_index: 0,
      multi_key_mode: 'random',
    },
  }),
  channelSchema.parse({
    id: 5,
    type: 1,
    key: 'sk-5',
    status: 2,
    name: 'Orphan',
    group: '',
    created_time: 0,
    test_time: 0,
    response_time: 0,
    balance_updated_time: 0,
  }),
]

// Rows the mocked backend serves; writes below update it like the server would.
let channels: Channel[] = []
const channelRequests: SearchRecord[] = []

function lastChannelRequest(): SearchRecord {
  const request = channelRequests.at(-1)
  if (!request) {
    throw new Error('No channel list request was made')
  }
  return request
}

// Returns the write spies so a test can assert the exact request it caused.
function renderTable() {
  // The backend filters by group; the mock mirrors that so the visible rows
  // follow the request.
  vi.spyOn(api, 'get').mockImplementation(async (url, config) => {
    if (url === '/api/channel') {
      const params = (config?.params ?? {}) as SearchRecord
      channelRequests.push(params)
      const items = channels.filter(
        (channel) =>
          params.group === undefined || channel.group === params.group
      )
      return {
        data: {
          success: true,
          data: {
            items,
            total: items.length,
            page: 1,
            page_size: 20,
            type_counts: { '1': 1, '14': 1 },
          },
        },
      }
    }
    if (url === '/api/group/') {
      return { data: { success: true, data: ['default', 'vip'] } }
    }
    if (url === '/api/channel/passthrough') {
      return {
        data: {
          success: true,
          data: {
            pass_through_request_enabled: false,
            pass_through_headers_enabled: false,
          },
        },
      }
    }
    return { data: { success: true, data: [] } }
  })
  const put = vi.spyOn(api, 'put').mockImplementation(async (url, body) => {
    const patch = body as Partial<Channel> & { id: number }
    if (url === '/api/channel/') {
      channels = channels.map((channel) =>
        channel.id === patch.id ? { ...channel, ...patch } : channel
      )
    }
    return { data: { success: true } }
  })
  const post = vi.spyOn(api, 'post').mockImplementation(async (url, body) => {
    const match = /^\/api\/channel\/(\d+)\/status$/.exec(String(url))
    if (match) {
      const status = (body as { status: number }).status
      channels = channels.map((channel) =>
        channel.id === Number(match[1]) ? { ...channel, status } : channel
      )
    }
    return { data: { success: true, data: true } }
  })

  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={queryClient}>
      <ChannelsProvider>
        <ChannelsTable />
      </ChannelsProvider>
    </QueryClientProvider>
  )
  return { put, post }
}

function rowOf(name: string): HTMLElement {
  const row = screen.getByText(name).closest('tr')
  if (!row) {
    throw new Error(`Missing table row for ${name}`)
  }
  return row
}

beforeEach(() => {
  localStorage.clear()
  localStorage.setItem('channels:view-mode', 'table')
  urlSearch.set({})
  channels = initialChannels
  channelRequests.length = 0
})

afterEach(() => {
  localStorage.clear()
})

describe('ChannelsTable group buttons', () => {
  test('clicking a group button requests and shows only that group', async () => {
    const user = userEvent.setup()
    renderTable()
    const groupBar = await screen.findByRole('group', { name: 'Group:' })
    const vipButton = await within(groupBar).findByRole('button', {
      name: 'vip',
    })
    await screen.findByText('OpenAI main')
    expect(
      within(groupBar).getByRole('button', { name: 'All' })
    ).toHaveAttribute('aria-pressed', 'true')

    await user.click(vipButton)

    await waitFor(() => expect(lastChannelRequest().group).toBe('vip'))
    expect(urlSearch.get().group).toEqual(['vip'])
    expect(vipButton).toHaveAttribute('aria-pressed', 'true')
    expect(
      within(groupBar).getByRole('button', { name: 'All' })
    ).toHaveAttribute('aria-pressed', 'false')
    await waitFor(() =>
      expect(screen.queryByText('OpenAI main')).not.toBeInTheDocument()
    )
    expect(screen.getByText('Claude vip')).toBeInTheDocument()
  })

  test('clicking All drops the group filter and keeps the status filter', async () => {
    const user = userEvent.setup()
    urlSearch.set({ group: ['vip'], status: ['enabled'] })
    renderTable()
    const groupBar = await screen.findByRole('group', { name: 'Group:' })
    await screen.findByText('Claude vip')
    expect(lastChannelRequest().group).toBe('vip')

    await user.click(within(groupBar).getByRole('button', { name: 'All' }))

    await waitFor(() => expect(lastChannelRequest().group).toBeUndefined())
    expect(lastChannelRequest().status).toBe('enabled')
    expect(urlSearch.get()).toEqual({ status: ['enabled'] })
    expect(await screen.findByText('OpenAI main')).toBeInTheDocument()
  })

  test('replaces the group popover filter with the button row', async () => {
    renderTable()
    await screen.findByRole('group', { name: 'Group:' })

    expect(
      screen.queryByRole('button', { name: 'Group' })
    ).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Type' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Status' })).toBeInTheDocument()
  })
})

describe('ChannelsTable cost / consumption / profit column', () => {
  test('replaces the balance and daily consumption columns', async () => {
    renderTable()
    await screen.findByText('OpenAI main')

    expect(
      screen.getByRole('button', {
        name: 'Cost / Consumption / Profit · Today',
      })
    ).toBeInTheDocument()
    expect(
      screen.queryByRole('columnheader', { name: /Used \/ Remaining/ })
    ).not.toBeInTheDocument()
    expect(
      screen.queryByRole('columnheader', { name: 'Consumed today' })
    ).not.toBeInTheDocument()
    expect(
      within(rowOf('OpenAI main')).getByTitle('Profit: $1.5')
    ).toBeVisible()
  })

  test('switching the header menu to all time shows total figures and is remembered', async () => {
    const user = userEvent.setup()
    renderTable()
    await screen.findByText('OpenAI main')

    await user.click(
      screen.getByRole('button', {
        name: 'Cost / Consumption / Profit · Today',
      })
    )
    const todayOption = await screen.findByRole('menuitemradio', {
      name: 'Today so far',
    })
    expect(todayOption).toHaveAttribute('aria-checked', 'true')
    expect(
      screen.getByRole('menuitemradio', { name: 'All time' })
    ).toHaveAttribute('aria-checked', 'false')
    await user.click(screen.getByRole('menuitemradio', { name: 'All time' }))

    expect(
      await screen.findByRole('button', {
        name: 'Cost / Consumption / Profit · All time',
      })
    ).toBeInTheDocument()
    const row = rowOf('OpenAI main')
    expect(within(row).getByTitle('Cost: $10')).toBeInTheDocument()
    expect(within(row).getByTitle('Consumption: $15')).toBeInTheDocument()
    expect(within(row).getByTitle('Profit: $5')).toBeInTheDocument()
    expect(localStorage.getItem('channels:quota-summary-scope')).toBe('total')
  })

  test('opens with the all-time scope saved by a previous visit', async () => {
    localStorage.setItem('channels:quota-summary-scope', 'total')
    renderTable()
    await screen.findByText('OpenAI main')

    expect(
      screen.getByRole('button', {
        name: 'Cost / Consumption / Profit · All time',
      })
    ).toBeInTheDocument()
    expect(within(rowOf('OpenAI main')).getByTitle('Profit: $5')).toBeVisible()
  })
})

describe('ChannelsTable channel type', () => {
  test('shows the type as an icon before the name instead of a type column', async () => {
    renderTable()
    await screen.findByText('OpenAI main')

    expect(
      screen.queryByRole('columnheader', { name: 'Type' })
    ).not.toBeInTheDocument()
    expect(
      within(rowOf('OpenAI main')).getByRole('img', { name: 'OpenAI' })
    ).toBeInTheDocument()
    expect(
      within(rowOf('Claude vip')).getByRole('img', { name: 'Anthropic' })
    ).toBeInTheDocument()
  })

  test('falls back to a generic icon for a type without a logo', async () => {
    renderTable()
    await screen.findByText('Legacy relay')

    const icon = within(rowOf('Legacy relay')).getByRole('img', {
      name: 'Unknown',
    })
    expect(icon.querySelector('svg.lucide-server')).not.toBeNull()
  })
})

describe('ChannelsTable row layout', () => {
  test('drops the status and groups columns', async () => {
    renderTable()
    await screen.findByText('OpenAI main')

    expect(
      screen.queryByRole('columnheader', { name: 'Status' })
    ).not.toBeInTheDocument()
    expect(
      screen.queryByRole('columnheader', { name: 'Groups' })
    ).not.toBeInTheDocument()
    // Priority, weight and cost ratio each own a column, so none of the three
    // chips has to share a cell with its neighbours. The resize handle's label
    // is part of the header's accessible name.
    for (const label of ['Priority', 'Weight', 'Cost ratio']) {
      expect(
        screen.getByRole('columnheader', { name: new RegExp(`^${label}`) })
      ).toBeInTheDocument()
    }
    expect(
      screen.queryByRole('columnheader', {
        name: /Priority \/ Weight \/ Cost ratio/,
      })
    ).not.toBeInTheDocument()
  })

  test('switching an enabled channel off posts the manual-disable status', async () => {
    const user = userEvent.setup()
    const { post } = renderTable()
    await screen.findByText('OpenAI main')
    const toggle = within(rowOf('OpenAI main')).getByRole('switch', {
      name: 'Enable',
    })
    expect(toggle).toBeChecked()

    await user.click(toggle)

    expect(post).toHaveBeenCalledWith(
      '/api/channel/1/status',
      { status: 2 },
      expect.anything()
    )
    await waitFor(() =>
      expect(
        within(rowOf('OpenAI main')).getByRole('switch', { name: 'Enable' })
      ).not.toBeChecked()
    )
  })

  test('switching a disabled channel on posts the enabled status', async () => {
    const user = userEvent.setup()
    const { post } = renderTable()
    await screen.findByText('Orphan')

    await user.click(
      within(rowOf('Orphan')).getByRole('switch', { name: 'Enable' })
    )

    expect(post).toHaveBeenCalledWith(
      '/api/channel/5/status',
      { status: 1 },
      expect.anything()
    )
  })

  test('marks an automatically disabled channel apart from a manual one', async () => {
    renderTable()
    await screen.findByText('Flaky pool')

    const autoRow = rowOf('Flaky pool')
    expect(
      within(autoRow).getByRole('switch', { name: 'Enable' })
    ).not.toBeChecked()
    expect(
      within(autoRow).getByRole('img', { name: 'Auto Disabled' })
    ).toBeInTheDocument()
    // Two of the three keys are still usable.
    expect(within(autoRow).getByText('2/3')).toBeInTheDocument()

    const manualRow = rowOf('Orphan')
    expect(
      within(manualRow).getByRole('switch', { name: 'Enable' })
    ).not.toBeChecked()
    expect(
      within(manualRow).queryByRole('img', { name: 'Auto Disabled' })
    ).not.toBeInTheDocument()
  })

  test('saving a priority chip sends only that field', async () => {
    const user = userEvent.setup()
    const { put } = renderTable()
    await screen.findByText('OpenAI main')

    await user.click(
      within(rowOf('OpenAI main')).getByRole('button', { name: 'Priority 0' })
    )
    const input = screen.getByRole('textbox', { name: 'Priority' })
    await user.clear(input)
    await user.type(input, '7{Enter}')

    expect(put).toHaveBeenCalledTimes(1)
    expect(put.mock.calls[0][1]).toEqual({ id: 1, priority: 7 })
  })

  test('ticking a group in the popover saves only the channel groups', async () => {
    const user = userEvent.setup()
    const { put } = renderTable()
    await screen.findByText('OpenAI main')

    await user.click(
      within(rowOf('OpenAI main')).getByRole('button', {
        name: 'Edit groups: default',
      })
    )
    expect(
      await screen.findByRole('textbox', { name: 'Search groups...' })
    ).toBeInTheDocument()
    expect(screen.getByRole('checkbox', { name: 'default' })).toBeChecked()
    await user.click(screen.getByRole('checkbox', { name: 'vip' }))

    await waitFor(() => expect(put).toHaveBeenCalledTimes(1))
    expect(put.mock.calls[0][0]).toBe('/api/channel/')
    expect(put.mock.calls[0][1]).toEqual({ id: 1, group: 'default,vip' })
    expect(put.mock.calls[0][1]).not.toHaveProperty('key')
    expect(put.mock.calls[0][1]).not.toHaveProperty('models')
    await waitFor(() =>
      expect(screen.getByRole('checkbox', { name: 'vip' })).toBeChecked()
    )
  })

  test('keeps the popover open after unticking the capsule that opened it', async () => {
    const user = userEvent.setup()
    channels = channels.map((channel) =>
      channel.id === 1 ? { ...channel, group: 'default,vip' } : channel
    )
    const { put } = renderTable()
    await screen.findByText('OpenAI main')

    await user.click(
      within(rowOf('OpenAI main')).getByRole('button', {
        name: 'Edit groups: vip',
      })
    )
    await user.click(await screen.findByRole('checkbox', { name: 'vip' }))

    await waitFor(() =>
      expect(screen.getByRole('checkbox', { name: 'vip' })).not.toBeChecked()
    )
    expect(put.mock.calls[0][1]).toEqual({ id: 1, group: 'default' })
    await user.click(screen.getByRole('checkbox', { name: 'vip' }))
    await waitFor(() => expect(put).toHaveBeenCalledTimes(2))
    expect(put.mock.calls[1][1]).toEqual({ id: 1, group: 'default,vip' })
  })

  test('filters the group grid by the search text', async () => {
    const user = userEvent.setup()
    renderTable()
    await screen.findByText('OpenAI main')

    await user.click(
      within(rowOf('OpenAI main')).getByRole('button', {
        name: 'Edit groups: default',
      })
    )
    await user.type(
      await screen.findByRole('textbox', { name: 'Search groups...' }),
      'vi'
    )

    expect(screen.getByRole('checkbox', { name: 'vip' })).toBeInTheDocument()
    expect(
      screen.queryByRole('checkbox', { name: 'default' })
    ).not.toBeInTheDocument()
  })

  test('refuses to untick the last group of a channel', async () => {
    const user = userEvent.setup()
    const { put } = renderTable()
    await screen.findByText('Claude vip')

    await user.click(
      within(rowOf('Claude vip')).getByRole('button', {
        name: 'Edit groups: vip',
      })
    )
    await user.click(await screen.findByRole('checkbox', { name: 'vip' }))

    expect(put).not.toHaveBeenCalled()
    expect(screen.getByRole('checkbox', { name: 'vip' })).toBeChecked()
  })

  test('offers an add-group button for a channel without groups', async () => {
    const user = userEvent.setup()
    const { put } = renderTable()
    await screen.findByText('Orphan')
    const row = rowOf('Orphan')
    expect(
      within(row).queryByRole('button', { name: /^Edit groups/ })
    ).not.toBeInTheDocument()

    await user.click(within(row).getByRole('button', { name: 'Add group' }))
    await user.click(await screen.findByRole('checkbox', { name: 'vip' }))

    await waitFor(() => expect(put).toHaveBeenCalledTimes(1))
    expect(put.mock.calls[0][1]).toEqual({ id: 5, group: 'vip' })
  })
})

describe('ChannelsTable group colours', () => {
  test('colours a group the same in every row and apart from the others', async () => {
    renderTable()
    await screen.findByText('OpenAI main')
    const capsuleClassName = createGroupCapsuleClassName(['default', 'vip'])

    // Three rows carry the default group; one carries vip.
    await waitFor(() =>
      expect(within(rowOf('OpenAI main')).getByText('default')).toHaveClass(
        capsuleClassName('default')
      )
    )
    expect(within(rowOf('Legacy relay')).getByText('default')).toHaveClass(
      capsuleClassName('default')
    )
    expect(within(rowOf('Flaky pool')).getByText('default')).toHaveClass(
      capsuleClassName('default')
    )
    expect(within(rowOf('Claude vip')).getByText('vip')).toHaveClass(
      capsuleClassName('vip')
    )
    expect(capsuleClassName('default')).not.toBe(capsuleClassName('vip'))
  })
})

describe('ChannelsTable card view', () => {
  test('shows groups as the same solid capsules next to the name', async () => {
    localStorage.setItem('channels:view-mode', 'card')
    renderTable()
    await screen.findByText('Claude vip')

    const capsule = screen.getByRole('button', { name: 'Edit groups: vip' })
    const nameRow = capsule.closest('[data-slot="channel-name-row"]')
    expect(nameRow).not.toBeNull()
    expect(nameRow).toHaveTextContent('Claude vip')
    // Coloured from the group list the page loaded (default, vip).
    const capsuleClassName = createGroupCapsuleClassName(['default', 'vip'])
    await waitFor(() =>
      expect(within(capsule).getByText('vip')).toHaveClass(
        capsuleClassName('vip'),
        'text-white'
      )
    )
    expect(capsuleClassName('vip')).not.toBe(capsuleClassName('default'))
  })

  test('shows all three routing chips in a card', async () => {
    localStorage.setItem('channels:view-mode', 'card')
    channels = channels.map((channel) =>
      channel.id === 1
        ? { ...channel, priority: 5, weight: 1, cost_ratio: 0.2 }
        : channel
    )
    renderTable()
    await screen.findByText('OpenAI main')

    // The table splits these into three columns; the card must not lose two of
    // them just because it only renders the first routing column.
    expect(screen.getByRole('button', { name: 'Priority 5' })).toBeVisible()
    expect(screen.getByRole('button', { name: 'Weight 1' })).toBeVisible()
    // The cost ratio stays read-only for anyone below super admin, so it is
    // rendered as a plain value chip.
    expect(screen.getByText('0.20x')).toBeVisible()
    expect(
      screen.queryByRole('button', { name: 'Cost ratio 0.20x' })
    ).not.toBeInTheDocument()
  })

  test('leaves card values at their natural width', async () => {
    localStorage.setItem('channels:view-mode', 'card')
    renderTable()
    await screen.findByText('OpenAI main')

    for (const slot of screen
      .getAllByTitle(/^(Cost|Consumption|Profit): /)
      .filter((node) => node.dataset.slot === 'aligned-value')) {
      expect(slot.style.minWidth).toBe('')
    }
  })

  test('shows today amounts with two decimals', async () => {
    localStorage.setItem('channels:view-mode', 'card')
    renderTable()
    await screen.findByText('OpenAI main')

    // Card view drops the currency symbol.
    expect(screen.getByTitle('Cost: 1')).toHaveTextContent(/^1\.00$/)
    expect(screen.getByTitle('Consumption: 2.5')).toHaveTextContent(/^2\.50$/)
    expect(screen.getByTitle('Profit: 1.5')).toHaveTextContent(/^1\.50$/)
  })
})

describe('ChannelsTable column alignment', () => {
  // Amount and chip value slots of one row, in column order.
  function amountSlots(name: string): HTMLElement[] {
    return within(rowOf(name))
      .getAllByTitle(/^(Cost|Consumption|Profit): /)
      .filter((slot) => slot.dataset.slot === 'aligned-value')
  }

  // Each routing field is a column of its own, so its chip is reached through
  // the cell that carries the field's column id.
  function chipSlots(name: string): HTMLElement[] {
    const row = rowOf(name)
    return ['priority', 'weight', 'cost_ratio'].map((columnId) => {
      const cell = row.querySelector(`[data-column-id="${columnId}"]`)
      const slot = cell?.querySelector<HTMLElement>(
        '[data-slot="aligned-value"]'
      )
      if (!slot) {
        throw new Error(`Missing ${columnId} slot in ${name}`)
      }
      return slot
    })
  }

  test('gives each amount slot the same right-aligned width on every row', async () => {
    channels = channels.map((channel) =>
      channel.id === 2
        ? {
            ...channel,
            // $2,823.47 against the $1.00 / $2.50 of OpenAI main.
            today_cost_quota: 2823.47 * 500_000,
            today_used_quota: 3000 * 500_000,
          }
        : channel
    )
    renderTable()
    await screen.findByText('OpenAI main')

    const small = amountSlots('OpenAI main')
    const large = amountSlots('Claude vip')
    const unavailable = amountSlots('Legacy relay')
    expect(small).toHaveLength(3)
    for (const [index, slot] of small.entries()) {
      expect(slot).toHaveClass('text-right', 'tabular-nums')
      expect(slot.style.minWidth).toBe(large[index].style.minWidth)
      expect(slot.style.minWidth).toBe(unavailable[index].style.minWidth)
    }
    expect(small[0]).toHaveTextContent('$1.00')
    expect(large[0]).toHaveTextContent('$2,823.47')
    expect(small[0].style.minWidth).toBe(`${'$2,823.47'.length}ch`)
    expect(unavailable[0]).toHaveTextContent('--')
  })

  test('editing a routing chip adds no element to the row it lives in', async () => {
    const user = userEvent.setup()
    renderTable()
    await screen.findByText('OpenAI main')

    const row = rowOf('OpenAI main')
    const cellsBefore = Array.from(
      row.querySelectorAll('[data-column-id]'),
      (cell) => cell.getAttribute('data-column-id')
    )
    const columnWidths = (): (string | null)[] =>
      Array.from(document.querySelectorAll('colgroup col'), (col) =>
        col.getAttribute('style')
      )
    const before = columnWidths()

    await user.click(
      within(row).getByRole('button', { name: 'Weight 0' })
    )
    expect(screen.getByRole('textbox', { name: 'Weight' })).toBeVisible()

    // The field overlays its chip instead of joining the row, so no cell is
    // added, removed or resized while the row is being edited.
    expect(
      Array.from(
        row.querySelectorAll('[data-column-id]'),
        (cell) => cell.getAttribute('data-column-id')
      )
    ).toEqual(cellsBefore)
    expect(columnWidths()).toEqual(before)
  })

  test('gives each routing chip the same right-aligned width on every row', async () => {
    channels = channels.map((channel) => {
      if (channel.id === 1) {
        return { ...channel, priority: 5, weight: 1, cost_ratio: 0.2 }
      }
      if (channel.id === 2) {
        return { ...channel, priority: 100, weight: 25, cost_ratio: 12.5 }
      }
      return channel
    })
    renderTable()
    await screen.findByText('OpenAI main')

    const narrow = chipSlots('OpenAI main')
    const wide = chipSlots('Claude vip')
    for (const [index, slot] of narrow.entries()) {
      expect(slot).toHaveClass('text-right', 'tabular-nums')
      expect(slot.style.minWidth).toBe(wide[index].style.minWidth)
    }
    expect(narrow.map((slot) => slot.textContent)).toEqual(['5', '1', '0.20x'])
    expect(narrow.map((slot) => slot.style.minWidth)).toEqual([
      '3ch',
      '2ch',
      `${'12.50x'.length}ch`,
    ])
  })

  test('recomputes the amount widths when the scope switches to all time', async () => {
    const user = userEvent.setup()
    channels = channels.map((channel) =>
      channel.id === 1
        ? { ...channel, total_used_quota: 23_456 * 500_000 }
        : channel
    )
    renderTable()
    await screen.findByText('OpenAI main')
    expect(amountSlots('OpenAI main')[1].style.minWidth).toBe(
      `${'$2.50'.length}ch`
    )

    await user.click(
      screen.getByRole('button', {
        name: 'Cost / Consumption / Profit · Today',
      })
    )
    await user.click(
      await screen.findByRole('menuitemradio', { name: 'All time' })
    )

    await waitFor(() =>
      expect(amountSlots('OpenAI main')[1]).toHaveTextContent('$23,456.00')
    )
    expect(amountSlots('OpenAI main')[1].style.minWidth).toBe(
      `${'$23,456.00'.length}ch`
    )
    expect(amountSlots('Claude vip')[1].style.minWidth).toBe(
      `${'$23,456.00'.length}ch`
    )
  })

  test('sizes the slots to the masked text when values are hidden', async () => {
    const user = userEvent.setup()
    renderTable()
    await screen.findByText('OpenAI main')

    await user.click(screen.getByRole('button', { name: 'Hide' }))

    // Hiding also masks channel names, so read the slots by title instead.
    await waitFor(() =>
      expect(screen.getAllByTitle('Consumption: ••••').length).toBeGreaterThan(
        0
      )
    )
    const cost = screen
      .getAllByTitle(/^Cost: /)
      .filter((slot) => slot.dataset.slot === 'aligned-value')
    for (const slot of cost) {
      // Masked amounts show "••••"; unavailable ones keep "--".
      expect(slot.style.minWidth).toBe('4ch')
    }
  })
})
