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
import { useEffect, useMemo } from 'react'

import { StatusBadge } from '@/components/status-badge'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { handleUpdateChannelField } from '../lib/channel-actions'
import { createChannelFieldUpdateScheduler } from '../lib/channel-field-update'
import { formatChannelCostRatio, isTagAggregateRow } from '../lib/channel-utils'
import type { Channel } from '../types'
import { NumericSpinnerInput } from './numeric-spinner-input'

export function ChannelCostRatioCell(props: { channel: Channel }) {
  const queryClient = useQueryClient()
  const isRoot = useAuthStore(
    (state) => state.auth.user?.role === ROLE.SUPER_ADMIN
  )
  const fieldUpdateScheduler = useMemo(
    () =>
      createChannelFieldUpdateScheduler((nextValue) => {
        void handleUpdateChannelField(
          props.channel.id,
          'cost_ratio',
          nextValue,
          queryClient
        )
      }),
    [props.channel.id, queryClient]
  )

  useEffect(() => () => fieldUpdateScheduler.flush(), [fieldUpdateScheduler])

  if (isTagAggregateRow(props.channel)) {
    const ratios = props.channel.children
      .map((child) => child.cost_ratio ?? 1)
      .filter((ratio) => Number.isFinite(ratio))
    if (ratios.length === 0) {
      return <span className='text-muted-foreground'>-</span>
    }
    const min = Math.min(...ratios)
    const max = Math.max(...ratios)
    const label =
      min === max
        ? formatChannelCostRatio(min)
        : `${formatChannelCostRatio(min)}-${formatChannelCostRatio(max)}`
    return (
      <StatusBadge label={label} variant='neutral' size='sm' copyable={false} />
    )
  }

  if (!isRoot) {
    return (
      <StatusBadge
        label={formatChannelCostRatio(props.channel.cost_ratio)}
        variant='info'
        size='sm'
        copyable={false}
      />
    )
  }

  return (
    <NumericSpinnerInput
      value={props.channel.cost_ratio ?? 1}
      onChange={fieldUpdateScheduler.schedule}
      onCommit={fieldUpdateScheduler.flush}
      min={0.01}
      max={1000}
      step={0.01}
    />
  )
}
