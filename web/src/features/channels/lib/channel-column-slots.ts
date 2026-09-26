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
import type { Channel, ChannelQuotaSummaryScope } from '../types'
import {
  getChannelQuotaSummary,
  type QuotaSummaryFormat,
} from './channel-quota-summary'
import { getChannelRoutingText } from './channel-routing-text'
import { isTagAggregateRow } from './channel-utils'

/**
 * Character widths of each value slot of the quota and routing columns,
 * taken from the longest text any row of the page shows in that slot.
 * Rendering every slot at this width, right-aligned in tabular digits, puts
 * decimal points, separators and chips in the same place on every row.
 */
export type ChannelColumnSlotWidths = {
  cost: number
  used: number
  profit: number
  priority: number
  weight: number
  costRatio: number
}

/** A table row may be a tag whose channels become rows once expanded. */
type ChannelRow = Channel & { children?: Channel[] }

// CJK and full-width characters take two cells of a monospace font.
const WIDE_CHARACTER =
  /[\u1100-\u115f\u2e80-\ua4cf\uac00-\ud7a3\uf900-\ufaff\ufe30-\ufe4f\uff00-\uff60\uffe0-\uffe6]/u

/** Width of a slot text in `ch`. */
function textWidth(text: string): number {
  let width = 0
  for (const char of text) {
    width += WIDE_CHARACTER.test(char) ? 2 : 1
  }
  return width
}

export function getChannelColumnSlotWidths(
  rows: readonly ChannelRow[],
  scope: ChannelQuotaSummaryScope,
  format: QuotaSummaryFormat
): ChannelColumnSlotWidths {
  const widths: ChannelColumnSlotWidths = {
    cost: 0,
    used: 0,
    profit: 0,
    priority: 0,
    weight: 0,
    costRatio: 0,
  }
  const visit = (row: ChannelRow) => {
    const summary = getChannelQuotaSummary(row, scope, format)
    const routing = getChannelRoutingText(row)
    widths.cost = Math.max(widths.cost, textWidth(summary.cost.display))
    widths.used = Math.max(widths.used, textWidth(summary.used.display))
    widths.profit = Math.max(widths.profit, textWidth(summary.profit.display))
    widths.priority = Math.max(widths.priority, textWidth(routing.priority))
    widths.weight = Math.max(widths.weight, textWidth(routing.weight))
    widths.costRatio = Math.max(widths.costRatio, textWidth(routing.costRatio))
    if (isTagAggregateRow(row)) {
      row.children.forEach(visit)
    }
  }
  rows.forEach(visit)
  return widths
}
