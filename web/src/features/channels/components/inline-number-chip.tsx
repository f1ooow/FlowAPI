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
import { AlignedValue } from './aligned-value'

/**
 * Quiet "label value" chip. The label is muted and the number carries the
 * weight, so a column of figures reads first and the chrome around them stays
 * out of the way. The box only appears on hover or while editing.
 */
const CHIP_CLASS_NAME =
  'inline-flex h-6 shrink-0 items-center gap-1.5 rounded-md border border-transparent px-1.5 text-xs leading-none whitespace-nowrap'

/**
 * Room the value box always keeps, in `ch`, so even a one-digit number has a
 * click target and every row's box ends up the same width.
 */
const MIN_VALUE_BOX_WIDTH = 3

/**
 * Read-only "label value" chip. `valueWidth` (in `ch`) gives the value a
 * fixed, right-aligned slot so chips line up across table rows.
 */
export function ValueChip(props: {
  label: string
  value: string
  valueWidth?: number
}) {
  return (
    <span className={CHIP_CLASS_NAME}>
      <span className='text-muted-foreground'>{props.label}</span>
      <span
        className='inline-block'
        style={{ minWidth: `${MIN_VALUE_BOX_WIDTH}ch` }}
      >
        <AlignedValue width={props.valueWidth} className='font-medium'>
          {props.value}
        </AlignedValue>
      </span>
    </span>
  )
}

type InlineNumberChipProps = {
  label: string
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
  /** Width of the value slot in `ch`; see {@link ValueChip}. */
  valueWidth?: number
  /** Omit to render the chip read-only. */
  onCommit?: (value: number) => void
}

/**
 * A "label value" chip that turns into an input when clicked. Enter or
 * leaving the field commits, Escape cancels, and the arrow keys step the value
 * before committing, so one edit always produces a single save.
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
        value={display}
        valueWidth={props.valueWidth}
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
        'bg-transparent transition-colors duration-100',
        editing && 'bg-muted/60 ring-ring/30 ring-1',
        !editing &&
          'hover:bg-muted/60 focus-visible:bg-muted/60 focus-visible:ring-ring/40 cursor-text focus-visible:ring-1'
      )}
    >
      <span className='text-muted-foreground'>{props.label}</span>
      {/* The field is absolutely positioned over the value box, which already
          reserves the widest the value can get, so editing adds nothing to the
          flow: the chip, its neighbours and the row keep their exact geometry. */}
      <span
        className='relative inline-flex'
        style={{ minWidth: `${MIN_VALUE_BOX_WIDTH}ch` }}
      >
        <AlignedValue
          width={props.valueWidth}
          className={cn(
            'font-medium transition-opacity duration-100',
            editing && 'invisible'
          )}
        >
          {display}
        </AlignedValue>
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
              if (pattern.test(raw) && (allowsNegative || !raw.startsWith('-'))) {
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
            className='ring-ring/60 bg-background/80 absolute inset-0 w-full rounded-sm text-right font-mono font-medium tabular-nums outline-none ring-2'
          />
        )}
      </span>
    </button>
  )
}
