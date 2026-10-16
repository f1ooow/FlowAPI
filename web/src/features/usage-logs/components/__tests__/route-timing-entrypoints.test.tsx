/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.
*/
import {
  flexRender,
  getCoreRowModel,
  useReactTable,
} from '@tanstack/react-table'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, test, vi } from 'vitest'

import { TooltipProvider } from '@/components/ui/tooltip'

import { usageLogSchema, type UsageLog } from '../../data/schema'
import { useCommonLogsColumns } from '../columns/common-logs-columns'
import { DetailsDialog } from '../dialogs/details-dialog'
import { UsageLogsProvider } from '../usage-logs-provider'

// Provider artwork is irrelevant to the channel and details entrypoints.
vi.mock('@lobehub/icons', () => ({}))

const log = usageLogSchema.parse({
  id: 1,
  user_id: 1,
  created_at: 1,
  type: 2,
  content: '',
  channel: 11,
  channel_name: 'Image provider',
  prompt_tokens: 0,
  completion_tokens: 0,
  quota: 0,
  other: JSON.stringify({
    admin_info: {
      route_history: [
        {
          channel_id: 11,
          attempt: 1,
          outcome: 'succeeded',
          status_code: 200,
          timing: {
            started_at_ms: 1700000000000,
            ended_at_ms: 1700000012300,
            start_offset_ms: 0,
            end_offset_ms: 12300,
            duration_ms: 12300,
          },
        },
      ],
    },
  }),
})

function ChannelCell(props: { isAdmin: boolean; log?: UsageLog }) {
  const columns = useCommonLogsColumns(props.isAdmin)
  const table = useReactTable({
    data: [props.log ?? log],
    columns,
    getCoreRowModel: getCoreRowModel(),
  })
  const cell = table
    .getRowModel()
    .rows[0]?.getAllCells()
    .find((entry) => entry.column.id === 'channel')
  return cell ? flexRender(cell.column.columnDef.cell, cell.getContext()) : null
}

describe('admin route timing entrypoints', () => {
  test('hides the retry badge for a direct success', () => {
    render(
      <TooltipProvider>
        <UsageLogsProvider>
          <ChannelCell isAdmin />
        </UsageLogsProvider>
      </TooltipProvider>
    )

    expect(
      screen.queryByRole('button', { name: 'Retry Chain' })
    ).not.toBeInTheDocument()
  })

  test('hides the retry badge for a terminal failure without retrying', () => {
    const failedLog = usageLogSchema.parse({
      ...log,
      other: JSON.stringify({
        admin_info: {
          route_history: [
            {
              channel_id: 11,
              attempt: 1,
              outcome: 'stopped',
              status_code: 400,
            },
          ],
        },
      }),
    })
    render(
      <TooltipProvider>
        <UsageLogsProvider>
          <ChannelCell isAdmin log={failedLog} />
        </UsageLogsProvider>
      </TooltipProvider>
    )

    expect(
      screen.queryByRole('button', { name: 'Retry Chain' })
    ).not.toBeInTheDocument()
  })

  test('shows the retry badge and chain for a retried request', async () => {
    const retriedLog = usageLogSchema.parse({
      ...log,
      other: JSON.stringify({
        admin_info: {
          route_history: [
            {
              channel_id: 11,
              attempt: 1,
              outcome: 'retrying_channel',
              status_code: 503,
            },
            {
              channel_id: 11,
              attempt: 2,
              outcome: 'succeeded',
              status_code: 200,
            },
          ],
        },
      }),
    })
    render(
      <TooltipProvider>
        <UsageLogsProvider>
          <ChannelCell isAdmin log={retriedLog} />
        </UsageLogsProvider>
      </TooltipProvider>
    )

    await userEvent
      .setup()
      .click(screen.getByRole('button', { name: 'Retry Chain' }))
    expect(screen.getByText('HTTP 503')).toBeInTheDocument()
    expect(screen.getByText('HTTP 200')).toBeInTheDocument()
  })

  test('shows direct-success timing in the admin details dialog', () => {
    render(
      <TooltipProvider>
        <DetailsDialog log={log} isAdmin open onOpenChange={vi.fn()} />
      </TooltipProvider>
    )
    expect(
      screen.getByRole('button', {
        name: 'Success recorded at +12.30 s · Attempt duration 12.30 s',
      })
    ).toBeInTheDocument()
  })

  test('hides route timing and the channel entrypoint from non-admins even if admin data is present', () => {
    render(
      <TooltipProvider>
        <UsageLogsProvider>
          <ChannelCell isAdmin={false} />
          <DetailsDialog
            log={log}
            isAdmin={false}
            open
            onOpenChange={vi.fn()}
          />
        </UsageLogsProvider>
      </TooltipProvider>
    )
    expect(
      screen.queryByRole('button', { name: 'Retry Chain' })
    ).not.toBeInTheDocument()
    expect(
      screen.queryByTestId('route-history-timeline')
    ).not.toBeInTheDocument()
  })
})
