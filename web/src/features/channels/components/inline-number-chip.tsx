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
import { useRef, useState } from 'react'

import { cn } from '@/lib/utils'

import { MIXED_ROUTING_VALUE } from '../lib/channel-routing-text'

/**
 * Quiet value chip. The number carries the weight and the box only appears on
 * hover or while editing, so a column of figures reads first.
 */
const CHIP_CLASS_NAME =
  'inline-flex h-6 shrink-0 items-center justify-end gap-1.5 rounded-md px-1.5 text-xs leading-none whitespace-nowrap'

/**
 * A table column names the figure in its header, so there the chip shows the
 * number alone and fills the column: the whole cell is the click target and the
 * editing field gets the column's full width. Its end padding is pulled back
 * into the cell padding, so the value ends exactly on the column's right edge,
 * the line the header title and every other row end on.
 */
const COLUMN_CHIP_CLASS_NAME = 'w-full -me-1.5'

/**
 * Room the value box always keeps, in `ch`, so even a one-digit number has a
 * click target.
 */
const MIN_VALUE_BOX_WIDTH = 3

const VALUE_CLASS_NAME = 'text-right font-mono font-medium tabular-nums'

type ChipLayoutProps = {
  /** Names the figure; shown in the chip only when `labelVisible`. */
  label: string
  /**
   * Whether the chip shows its label. Off (the default) in a table column,
   * whose header already names the figure; on where chips sit side by side
   * (a card).
   */
  labelVisible?: boolean
}

/** Read-only chip of a figure that cannot be edited here. */
function ValueChip(props: ChipLayoutProps & { value: string }) {
  return (
    <span
      className={cn(
        CHIP_CLASS_NAME,
        !props.labelVisible && COLUMN_CHIP_CLASS_NAME
      )}
    >
      {props.labelVisible && (
        <span className='text-muted-foreground'>{props.label}</span>
      )}
      <span
        className='inline-flex justify-end'
        style={{ minWidth: `${MIN_VALUE_BOX_WIDTH}ch` }}
      >
        <span data-slot='aligned-number' className={VALUE_CLASS_NAME}>
          {props.value}
        </span>
      </span>
    </span>
  )
}

type InlineNumberChipProps = ChipLayoutProps & {
  /** `null` is a mixed value (e.g. a tag whose channels disagree). */
  value: number | null
  formatValue?: (value: number) => string
  /**
   * Text to show instead of the formatted value, for a figure that is not a
   * single number (e.g. a tag's cost ratio range). Editing is unavailable
   * whenever this is set.
   */
  displayText?: string
  min: number
  max?: number
  /** An integer step only accepts integers; a fractional one accepts decimals. */
  step?: number
  /** Omit to render the chip read-only. */
  onCommit?: (value: number) => void
}

/**
 * A number chip that turns into an input when clicked. Enter or leaving the
 * field commits, Escape cancels, and the arrow keys step the value before
 * committing, so one edit always produces a single save.
 */
export function InlineNumberChip(props: InlineNumberChipProps) {
  const [draft, setDraft] = useState<string | null>(null)
  const cancelledRef = useRef(false)
  // The chip stays the focusable element across an edit, so leaving the field
  // with Enter or Escape puts the focus back on it instead of dropping it.
  const chipRef = useRef<HTMLButtonElement>(null)
  const keyboardExitRef = useRef(false)
  const step = props.step ?? 1
  const display =
    props.displayText ??
    (props.value == null
      ? MIXED_ROUTING_VALUE
      : (props.formatValue?.(props.value) ?? String(props.value)))
  const onCommit = props.onCommit

  // A value that is not a single number has nothing to edit.
  if (!onCommit || props.displayText !== undefined) {
    return (
      <ValueChip
        label={props.label}
        labelVisible={props.labelVisible}
        value={display}
      />
    )
  }
  const clamp = (value: number) => {
    const bounded = Math.max(props.min, value)
    return props.max === undefined ? bounded : Math.min(props.max, bounded)
  }
  const roundToStep = (value: number) => {
    const decimals = step.toString().split('.')[1]?.length ?? 0
    return Number(value.toFixed(decimals))
  }

  const finishEditing = () => {
    const text = draft
    setDraft(null)
    // Enter and Escape mean "I am done with this field", so the focus belongs
    // back on the chip. A blur from clicking elsewhere leaves it where the
    // pointer put it.
    if (keyboardExitRef.current) {
      keyboardExitRef.current = false
      chipRef.current?.focus()
    }
    if (cancelledRef.current) {
      cancelledRef.current = false
      return
    }
    const parsed = Number(text)
    if (text === null || text.trim() === '' || !Number.isFinite(parsed)) {
      return
    }
    const next = clamp(roundToStep(parsed))
    if (next !== props.value) {
      onCommit(next)
    }
  }

  const allowsDecimals = !Number.isInteger(step)
  const allowsNegative = props.min < 0
  const editing = draft !== null

  return (
    // One element for both states: the chip never unmounts, so entering an edit
    // cannot move focus, height or width, and returning to the display state
    // leaves the focus on the chip.
    <button
      ref={chipRef}
      type='button'
      aria-label={`${props.label} ${display}`}
      onClick={(event) => {
        event.stopPropagation()
        if (!editing) {
          setDraft(props.value == null ? '' : String(props.value))
        }
      }}
      className={cn(
        CHIP_CLASS_NAME,
        !props.labelVisible && COLUMN_CHIP_CLASS_NAME,
        'bg-transparent transition-colors duration-100',
        editing && 'bg-muted/60 ring-ring/30 ring-1',
        !editing &&
          'hover:bg-muted/60 focus-visible:bg-muted/60 focus-visible:ring-ring/40 cursor-text focus-visible:ring-1'
      )}
    >
      {props.labelVisible && (
        <span className='text-muted-foreground'>{props.label}</span>
      )}
      {/* The field is absolutely positioned over the value box, so editing
          adds nothing to the flow: the chip, its neighbours and the row keep
          their exact geometry. */}
      <span
        className='relative inline-flex flex-1 justify-end'
        style={{ minWidth: `${MIN_VALUE_BOX_WIDTH}ch` }}
      >
        <span
          data-slot='aligned-number'
          className={cn(
            VALUE_CLASS_NAME,
            'transition-opacity duration-100',
            editing && 'invisible'
          )}
        >
          {display}
        </span>
        {editing && (
          <input
            aria-label={props.label}
            value={draft}
            inputMode={allowsDecimals ? 'decimal' : 'numeric'}
            autoFocus
            onFocus={(event) => event.currentTarget.select()}
            onClick={(event) => event.stopPropagation()}
            onChange={(event) => {
              const raw = event.target.value
              const pattern = allowsDecimals ? /^-?\d*(?:\.\d*)?$/ : /^-?\d*$/
              if (
                pattern.test(raw) &&
                (allowsNegative || !raw.startsWith('-'))
              ) {
                setDraft(raw)
              }
            }}
            onKeyDown={(event) => {
              if (event.key === 'Enter' || event.key === 'Escape') {
                event.preventDefault()
                keyboardExitRef.current = true
                cancelledRef.current = event.key === 'Escape'
                event.currentTarget.blur()
              } else if (event.key === 'ArrowUp' || event.key === 'ArrowDown') {
                event.preventDefault()
                const current = Number(draft) || 0
                const delta = event.key === 'ArrowUp' ? step : -step
                setDraft(String(clamp(roundToStep(current + delta))))
              }
            }}
            onBlur={finishEditing}
            className='ring-ring/60 bg-background/80 absolute inset-0 w-full rounded-sm text-right font-mono font-medium tabular-nums ring-2 outline-none'
          />
        )}
      </span>
    </button>
  )
}
