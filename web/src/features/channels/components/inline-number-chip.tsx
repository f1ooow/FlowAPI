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

const CHIP_CLASS_NAME =
  'inline-flex h-6 shrink-0 items-center gap-1 rounded-md border px-1.5 text-xs leading-none whitespace-nowrap'

/** Read-only "label value" chip. */
export function ValueChip(props: { label: string; value: string }) {
  return (
    <span className={cn(CHIP_CLASS_NAME, 'bg-muted/40')}>
      <span className='text-muted-foreground'>{props.label}</span>
      <span className='font-mono font-medium tabular-nums'>{props.value}</span>
    </span>
  )
}

type InlineNumberChipProps = {
  label: string
  /** `null` is a mixed value (e.g. a tag whose channels disagree). */
  value: number | null
  formatValue?: (value: number) => string
  min: number
  max?: number
  /** An integer step only accepts integers; a fractional one accepts decimals. */
  step?: number
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
  const step = props.step ?? 1
  const display =
    props.value == null
      ? '-'
      : (props.formatValue?.(props.value) ?? String(props.value))

  if (!props.onCommit) {
    return <ValueChip label={props.label} value={display} />
  }

  const onCommit = props.onCommit
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

  if (draft === null) {
    return (
      <button
        type='button'
        aria-label={`${props.label} ${display}`}
        onClick={(event) => {
          event.stopPropagation()
          setDraft(props.value == null ? '' : String(props.value))
        }}
        className={cn(
          CHIP_CLASS_NAME,
          'bg-muted/40 hover:bg-muted hover:border-foreground/20 cursor-text transition-colors'
        )}
      >
        <span className='text-muted-foreground'>{props.label}</span>
        <span className='font-mono font-medium tabular-nums'>{display}</span>
      </button>
    )
  }

  const allowsDecimals = !Number.isInteger(step)
  const allowsNegative = props.min < 0

  return (
    <span className={cn(CHIP_CLASS_NAME, 'ring-primary/30 bg-muted ring-1')}>
      <span className='text-muted-foreground'>{props.label}</span>
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
          if (event.key === 'Enter') {
            event.preventDefault()
            event.currentTarget.blur()
          } else if (event.key === 'Escape') {
            cancelledRef.current = true
            event.currentTarget.blur()
          } else if (event.key === 'ArrowUp' || event.key === 'ArrowDown') {
            event.preventDefault()
            const current = Number(draft) || 0
            const delta = event.key === 'ArrowUp' ? step : -step
            setDraft(String(clamp(roundToStep(current + delta))))
          }
        }}
        onBlur={finishEditing}
        className='w-12 bg-transparent font-mono font-medium tabular-nums outline-none'
      />
    </span>
  )
}
