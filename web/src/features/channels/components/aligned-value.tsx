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
import { cn } from '@/lib/utils'

/**
 * Right-aligned, tabular-digit value slot. With a width in `ch`, every row
 * gives the slot the same room, so decimal points line up down the column;
 * without one the slot hugs its text.
 */
export function AlignedValue(props: {
  width: number | undefined
  className?: string
  title?: string
  children: string
}) {
  return (
    <span
      data-slot='aligned-value'
      title={props.title}
      style={props.width ? { minWidth: `${props.width}ch` } : undefined}
      className={cn(
        'inline-block text-right font-mono tabular-nums whitespace-nowrap',
        props.className
      )}
    >
      {props.children}
    </span>
  )
}
