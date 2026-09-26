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
import { flexRender, type Row } from '@tanstack/react-table'
import { memo } from 'react'
import { useTranslation } from 'react-i18next'

import { cn } from '@/lib/utils'

import { getChannelQuotaSummaryTitle, isTagAggregateRow } from '../lib'
import type { Channel } from '../types'
import { ChannelRowActionsLayoutContext } from './channel-row-actions-context'
import { useChannels } from './channels-provider'

const SENSITIVE_MASK = '••••'

/**
 * Bespoke channel card for the card view. Reuses every column's existing cell
 * renderer via `flexRender`, so the table's information and interactions are
 * preserved: row selection, provider/multi-key/IO.NET type badge, id,
 * name/remark + warning icons, editable group capsules, the status switch,
 * priority/weight/cost-ratio chips, cost/consumption/profit, response/test
 * times, tag expand-collapse, and the per-row (or per-tag) actions menu.
 */
function ChannelCardComponent({
  row,
  isSelected,
}: {
  row: Row<Channel>
  isSelected: boolean
}) {
  const { t } = useTranslation()
  const { sensitiveVisible, quotaSummaryScope } = useChannels()
  const isTagRow = isTagAggregateRow(row.original)
  const cells = row.getAllCells()

  const renderCell = (id: string) => {
    const cell = cells.find((c) => c.column.id === id)
    if (!cell || !cell.column.columnDef.cell) {
      return null
    }
    return flexRender(cell.column.columnDef.cell, cell.getContext())
  }

  // The quota cell follows the scope picked in the table header, so the
  // label names that scope too.
  const quotaSummaryLabel = getChannelQuotaSummaryTitle(quotaSummaryScope, t)

  const selectCell = renderCell('select')
  const typeCell = renderCell('channel_type')
  const nameCell = renderCell('name')
  const actionsCell = renderCell('actions')
  const routingCell = renderCell('priority')
  const quotaSummaryCell = renderCell('quota_summary')
  const responseCell = renderCell('response_time')
  const testCell = renderCell('test_time')

  const labelClass = 'text-muted-foreground text-[11px] font-medium select-none'

  return (
    <ChannelRowActionsLayoutContext.Provider value='card'>
      <div
        data-state={isSelected ? 'selected' : undefined}
        className='flex flex-col gap-3'
      >
        {/* Row 1: selection + type, with the status switch + actions menu */}
        <div className='flex items-center justify-between gap-2'>
          <div className='flex min-w-0 flex-1 items-center gap-2'>
            {!isTagRow && selectCell && (
              <span className='shrink-0'>{selectCell}</span>
            )}
            <div className='min-w-0 overflow-hidden'>{typeCell}</div>
          </div>
          <div className='flex shrink-0 items-center gap-1.5'>
            {actionsCell}
          </div>
        </div>

        {/* Identity: id, name, remark and the editable group capsules */}
        <div className='min-w-0 text-sm'>
          {!isTagRow && (
            <div className={labelClass}>
              #{sensitiveVisible ? row.original.id : SENSITIVE_MASK}
            </div>
          )}
          {nameCell}
        </div>

        {/* Routing chips: priority, weight and cost ratio */}
        {routingCell}

        {/* Usage paired with the response and last-tested times */}
        <div className='flex items-start justify-between gap-3'>
          <div className='min-w-0 flex-1 overflow-hidden'>
            <div className={cn('mb-1', labelClass)}>{quotaSummaryLabel}</div>
            <div className='min-w-0 overflow-hidden text-sm'>
              {quotaSummaryCell ?? (
                <span className='text-muted-foreground'>--</span>
              )}
            </div>
          </div>
          <div className='grid shrink-0 grid-cols-[auto_auto] items-center gap-x-3 gap-y-1'>
            <span className={labelClass}>{t('Response')}</span>
            <span className={labelClass}>{t('Last Tested')}</span>
            <div className='overflow-hidden text-sm'>
              {responseCell ?? <span className='text-muted-foreground'>-</span>}
            </div>
            <div className='overflow-hidden text-sm'>
              {testCell ?? <span className='text-muted-foreground'>-</span>}
            </div>
          </div>
        </div>
      </div>
    </ChannelRowActionsLayoutContext.Provider>
  )
}

/**
 * Memoized so each card only re-renders when its own react-table row reference
 * changes, instead of every card re-rendering whenever the parent table state
 * (filters, pagination, sensitive toggle, etc.) updates.
 */
export const ChannelCard = memo(ChannelCardComponent)
