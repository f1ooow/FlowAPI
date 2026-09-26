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
import { createContext } from 'react'

import type { ChannelColumnSlotWidths } from '../lib/channel-column-slots'

/**
 * Slot widths the channel table computes from its current page, so the
 * quota and routing cells of every row line up. Context rather than a
 * column prop: a width change reaches the cells without rebuilding the
 * column definitions, and it reaches them through memoized rows.
 * `null` leaves every slot at its natural width.
 */
export const ChannelColumnSlotsContext =
  createContext<ChannelColumnSlotWidths | null>(null)
