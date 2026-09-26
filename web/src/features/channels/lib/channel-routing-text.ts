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
import type { Channel } from '../types'
import { formatChannelCostRatio, isTagAggregateRow } from './channel-utils'

/** Shown for a tag whose channels disagree on a value. */
export const MIXED_ROUTING_VALUE = '-'

export type ChannelRoutingText = {
  priority: string
  weight: string
  costRatio: string
}

/**
 * The priority, weight and cost ratio texts a channel row shows. A tag row
 * reads the values its channels share and shows its cost ratio as the range
 * of its channels. The cell renders these texts and the table sizes its
 * aligned chips from them, so both always agree.
 */
export function getChannelRoutingText(channel: Channel): ChannelRoutingText {
  if (!isTagAggregateRow(channel)) {
    return {
      priority: String(channel.priority ?? 0),
      weight: String(channel.weight ?? 0),
      costRatio: formatChannelCostRatio(channel.cost_ratio),
    }
  }

  const ratios = channel.children
    .map((child) => child.cost_ratio ?? 1)
    .filter((ratio) => Number.isFinite(ratio))
  // No channel reported a ratio; the chip shows the mixed marker.
  let costRatio = MIXED_ROUTING_VALUE
  if (ratios.length > 0) {
    const min = Math.min(...ratios)
    const max = Math.max(...ratios)
    costRatio =
      min === max
        ? formatChannelCostRatio(min)
        : `${formatChannelCostRatio(min)}-${formatChannelCostRatio(max)}`
  }
  return {
    priority:
      channel.priority == null ? MIXED_ROUTING_VALUE : String(channel.priority),
    weight:
      channel.weight == null ? MIXED_ROUTING_VALUE : String(channel.weight),
    costRatio,
  }
}
