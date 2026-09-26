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
import { useState } from 'react'
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
import type { Channel } from '../types'
import { InlineNumberChip, ValueChip } from './inline-number-chip'

type TagFieldUpdate = { field: 'priority' | 'weight'; value: number }

/**
 * Priority, weight and cost ratio of a channel as editable chips. A tag row
 * edits priority and weight for every channel in the tag after confirmation;
 * its cost ratio is shown as the range of its channels. Only super admins may
 * change a cost ratio, matching the backend rule.
 */
export function ChannelRoutingCell(props: { channel: Channel }) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const isRoot = useAuthStore(
    (state) => state.auth.user?.role === ROLE.SUPER_ADMIN
  )
  const [tagUpdate, setTagUpdate] = useState<TagFieldUpdate | null>(null)
  const channel = props.channel

  if (isTagAggregateRow(channel)) {
    const tag = channel.tag || ''
    const ratios = channel.children
      .map((child) => child.cost_ratio ?? 1)
      .filter((ratio) => Number.isFinite(ratio))
    let ratioLabel = '-'
    if (ratios.length > 0) {
      const min = Math.min(...ratios)
      const max = Math.max(...ratios)
      ratioLabel =
        min === max
          ? formatChannelCostRatio(min)
          : `${formatChannelCostRatio(min)}-${formatChannelCostRatio(max)}`
    }
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
      <div className='flex flex-wrap items-center gap-1'>
        <InlineNumberChip
          label={t('Priority')}
          value={channel.priority ?? null}
          min={-999}
          onCommit={(value) => setTagUpdate({ field: 'priority', value })}
        />
        <InlineNumberChip
          label={t('Weight')}
          value={channel.weight ?? null}
          min={0}
          onCommit={(value) => setTagUpdate({ field: 'weight', value })}
        />
        <ValueChip label={t('Cost ratio')} value={ratioLabel} />
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
    <div className='flex flex-wrap items-center gap-1'>
      <InlineNumberChip
        label={t('Priority')}
        value={channel.priority ?? 0}
        min={-999}
        onCommit={(value) =>
          void handleUpdateChannelField(
            channel.id,
            'priority',
            value,
            queryClient
          )
        }
      />
      <InlineNumberChip
        label={t('Weight')}
        value={channel.weight ?? 0}
        min={0}
        onCommit={(value) =>
          void handleUpdateChannelField(
            channel.id,
            'weight',
            value,
            queryClient
          )
        }
      />
      <InlineNumberChip
        label={t('Cost ratio')}
        value={channel.cost_ratio ?? 1}
        formatValue={formatChannelCostRatio}
        min={0.01}
        max={1000}
        step={0.01}
        onCommit={
          isRoot
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
    </div>
  )
}
