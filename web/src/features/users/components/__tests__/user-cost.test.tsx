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
import {
  flexRender,
  getCoreRowModel,
  useReactTable,
} from '@tanstack/react-table'
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, test } from 'vitest'

import { CardRowContent, DataTableViewOptions } from '@/components/data-table'
import { TooltipProvider } from '@/components/ui/tooltip'
import {
  DEFAULT_CURRENCY_CONFIG,
  useSystemConfigStore,
} from '@/stores/system-config-store'

import { userListItemSchema, type UserListItem } from '../../types'
import { useUsersColumns } from '../users-columns'

function UserAccountingTable(props: {
  costQuota: number | null
  incomplete?: boolean
  mobile?: boolean
}) {
  const columns = useUsersColumns().filter(
    (column) => column.id === 'quota' || column.id === 'cost_quota'
  )
  const table = useReactTable({
    data: [
      userListItemSchema.parse({
        id: 1,
        username: 'alice',
        display_name: 'Alice',
        quota: 1_000_000,
        used_quota: 250_000,
        request_count: 1,
        group: 'default',
        status: 1,
        role: 1,
        cost_quota: props.costQuota,
        cost_incomplete: props.incomplete ?? false,
      }),
    ],
    columns,
    getCoreRowModel: getCoreRowModel(),
  })

  return (
    <TooltipProvider>
      <DataTableViewOptions table={table} />
      {props.mobile ? (
        table
          .getRowModel()
          .rows.map((row) => (
            <CardRowContent<UserListItem> key={row.id} row={row} compact />
          ))
      ) : (
        <table aria-label='Users'>
          <thead>
            {table.getHeaderGroups().map((group) => (
              <tr key={group.id}>
                {group.headers.map((header) => (
                  <th key={header.id}>
                    {flexRender(
                      header.column.columnDef.header,
                      header.getContext()
                    )}
                  </th>
                ))}
              </tr>
            ))}
          </thead>
          <tbody>
            {table.getRowModel().rows.map((row) => (
              <tr key={row.id}>
                {row.getVisibleCells().map((cell) => (
                  <td key={cell.id}>
                    {flexRender(cell.column.columnDef.cell, cell.getContext())}
                  </td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </TooltipProvider>
  )
}

beforeEach(() => {
  useSystemConfigStore.getState().setConfig({
    currency: {
      ...DEFAULT_CURRENCY_CONFIG,
      quotaDisplayType: 'CNY',
      usdExchangeRate: 7,
    },
  })
})

afterEach(() => {
  useSystemConfigStore.getState().setConfig({
    currency: DEFAULT_CURRENCY_CONFIG,
  })
})

describe('admin user cost column', () => {
  test('shows upstream cost in the configured currency beside the unchanged quota column', () => {
    render(<UserAccountingTable costQuota={625_000} />)

    expect(
      screen.getAllByRole('columnheader').map((header) => header.textContent)
    ).toEqual(['Quota', 'Upstream Cost'])
    expect(screen.getByText('¥8.75')).toBeInTheDocument()
    expect(screen.getByText('¥14')).toBeInTheDocument()
    expect(screen.getByText('¥17.5')).toBeInTheDocument()
  })

  test('shows plain cost without explanatory controls when cost history is incomplete', async () => {
    render(<UserAccountingTable costQuota={625_000} incomplete />)

    const cost = screen.getByRole('cell', { name: /^¥8.75/ })
    expect(cost).toHaveTextContent(/^¥8.75$/)
    expect(within(cost).queryByRole('button')).not.toBeInTheDocument()
    await userEvent.setup().hover(cost)
    expect(screen.queryByRole('tooltip')).not.toBeInTheDocument()
  })

  test('shows unavailable cost as a plain dash rather than a monetary zero', () => {
    render(<UserAccountingTable costQuota={null} incomplete />)

    const unavailable = screen.getByRole('cell', { name: '-' })
    expect(unavailable).toHaveTextContent(/^-$/)
    expect(within(unavailable).queryByRole('button')).not.toBeInTheDocument()
    expect(screen.queryByText('¥0')).not.toBeInTheDocument()
  })

  test('keeps a verified zero distinct from unavailable cost without annotations', () => {
    render(<UserAccountingTable costQuota={0} />)

    const cost = screen.getByRole('cell', { name: '¥0' })
    expect(cost).toHaveTextContent(/^¥0$/)
    expect(within(cost).queryByRole('button')).not.toBeInTheDocument()
  })

  test('allows hiding the cost column without hiding or changing user quota', async () => {
    render(<UserAccountingTable costQuota={625_000} />)
    const user = userEvent.setup()

    await user.click(screen.getByRole('button', { name: 'View' }))
    await user.click(
      screen.getByRole('menuitemcheckbox', { name: 'Upstream Cost' })
    )

    expect(
      screen.queryByRole('columnheader', { name: 'Upstream Cost' })
    ).not.toBeInTheDocument()
    expect(screen.queryByText('¥8.75')).not.toBeInTheDocument()
    expect(screen.getByText('¥14')).toBeInTheDocument()
  })

  test('includes the cost label and amount in the mobile card layout', () => {
    render(<UserAccountingTable costQuota={625_000} mobile />)

    expect(screen.getByText('Upstream Cost')).toBeInTheDocument()
    expect(screen.getByText('¥8.75')).toBeInTheDocument()
    expect(screen.getByText('Quota')).toBeInTheDocument()
    expect(screen.getByText('Used: ¥3.5')).toBeVisible()
  })
})
