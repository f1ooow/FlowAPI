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
import { useQueryClient } from '@tanstack/react-query'
import { useContext, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import {
  formatChannelCostRatio,
  handleUpdateChannelField,
  handleUpdateTagField,
  isTagAggregateRow,
} from '../lib'
import { getChannelRoutingText } from '../lib/channel-routing-text'
import type { Channel } from '../types'
import { ChannelColumnSlotsContext } from './channel-column-slots-context'
import { ChannelRowActionsLayoutContext } from './channel-row-actions-context'
import { InlineNumberChip } from './inline-number-chip'

/** The routing figure a single column of the table edits. */
export type ChannelRoutingField = 'priority' | 'weight' | 'costRatio'

type TagFieldUpdate = { field: 'priority' | 'weight'; value: number }

/**
 * Priority, weight or cost ratio of a channel as one editable chip, sized for
 * one table column. The three fields are separate columns whose widths the
 * table fixes, so a chip never shares a cell with its neighbours and never
 * wraps. A tag row edits priority and weight for every channel in the tag
 * after confirmation; its cost ratio is the range of its channels. Only super
 * admins may change a cost ratio, matching the backend rule, so for everyone
 * else the chip is read-only.
 *
 * The confirming dialog is owned by the row, not by a column, so a tag edit
 * shows one dialog rather than one per column.
 */
export function ChannelRoutingValueCell(props: {
  channel: Channel
  field: ChannelRoutingField
  /** Called with the confirmed value of a tag-wide priority or weight edit. */
  onTagUpdate?: (field: 'priority' | 'weight', value: number) => void
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const layout = useContext(ChannelRowActionsLayoutContext)
  const slotWidths = useContext(ChannelColumnSlotsContext)
  const isRoot = useAuthStore(
    (state) => state.auth.user?.role === ROLE.SUPER_ADMIN
  )
  const channel = props.channel
  // In edit mode the chip overlays its value box instead of growing, so the
  // shared slot widths only decide how wide each value box is; without them
  // (cards) the box hugs its own text.
  const widths = layout === 'table' ? slotWidths : null
  const isTagRow = isTagAggregateRow(channel)
  const routing = getChannelRoutingText(channel)

  if (props.field === 'costRatio') {
    return (
      <InlineNumberChip
        label={t('Cost ratio')}
        // A tag row shows the range of its channels; a channel shows its own
        // ratio. Both texts come from the same helper the slot width uses.
        value={isTagRow ? null : (channel.cost_ratio ?? 1)}
        formatValue={formatChannelCostRatio}
        displayText={isTagRow ? routing.costRatio : undefined}
        valueWidth={widths?.costRatio}
        min={0.01}
        max={1000}
        step={0.01}
        onCommit={
          isRoot && !isTagRow
            ? (value) =>
                void handleUpdateChannelField(
                  channel.id,
                  'cost_ratio',
                  value,
                  queryClient
                )
            : undefined
        }
      />
    )
  }

  const onTagUpdate = props.onTagUpdate
  const label = t(props.field === 'priority' ? 'Priority' : 'Weight')
  // A tag row carries the value its channels share and null when they differ;
  // null is the mixed marker, so it must not fall back to a number.
  const rawValue = channel[props.field]
  const value =
    rawValue == null && isTagRow
      ? null
      : ((rawValue as number | null | undefined) ?? 0)
  // The batch handler is what makes a tag row's figure editable; without one
  // (a card, which lays the row's fields out together) it stays a read-only range.
  const batchField = props.field as 'priority' | 'weight'
  let onCommit: ((next: number) => void) | undefined
  if (isTagRow) {
    onCommit = onTagUpdate ? (next) => onTagUpdate(batchField, next) : undefined
  } else {
    onCommit = (next) =>
      void handleUpdateChannelField(channel.id, props.field, next, queryClient)
  }

  return (
    <InlineNumberChip
      label={label}
      value={value}
      valueWidth={widths?.[props.field]}
      min={props.field === 'priority' ? -999 : 0}
      onCommit={onCommit}
    />
  )
}

/**
 * Priority, weight and cost ratio of a channel as three editable chips, laid
 * out to sit in one row of the card view. The table renders one
 * {@link ChannelRoutingValueCell} per column instead, which fixes each chip's
 * width and keeps the numbers aligned down the column.
 */
export function ChannelRoutingCell(props: { channel: Channel }) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [tagUpdate, setTagUpdate] = useState<TagFieldUpdate | null>(null)
  const channel = props.channel

  if (isTagAggregateRow(channel)) {
    const tag = channel.tag || ''
    const confirmDescription =
      tagUpdate?.field === 'priority'
        ? t(
            'This will update the priority to {{value}} for all {{count}} channel(s) with tag "{{tag}}". Continue?',
            { value: tagUpdate.value, count: channel.children.length, tag }
          )
        : t(
            'This will update the weight to {{value}} for all {{count}} channel(s) with tag "{{tag}}". Continue?',
            { value: tagUpdate?.value, count: channel.children.length, tag }
          )

    return (
      <div className='flex items-center gap-2'>
        <ChannelRoutingValueCell
          channel={channel}
          field='priority'
          onTagUpdate={(field, value) => setTagUpdate({ field, value })}
        />
        <ChannelRoutingValueCell
          channel={channel}
          field='weight'
          onTagUpdate={(field, value) => setTagUpdate({ field, value })}
        />
        <ChannelRoutingValueCell channel={channel} field='costRatio' />
        <ConfirmDialog
          open={tagUpdate !== null}
          onOpenChange={(open) => {
            if (!open) {
              setTagUpdate(null)
            }
          }}
          title={t('Confirm Batch Update')}
          desc={confirmDescription}
          confirmText={t('Update')}
          handleConfirm={() => {
            if (tagUpdate) {
              handleUpdateTagField(
                tag,
                tagUpdate.field,
                tagUpdate.value,
                queryClient
              )
            }
            setTagUpdate(null)
          }}
        />
      </div>
    )
  }

  return (
    <div className='flex items-center gap-2'>
      <ChannelRoutingValueCell channel={channel} field='priority' />
      <ChannelRoutingValueCell channel={channel} field='weight' />
      <ChannelRoutingValueCell channel={channel} field='costRatio' />
    </div>
  )
}
