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
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, test, vi } from 'vitest'

import { InlineNumberChip } from '../inline-number-chip'

describe('InlineNumberChip', () => {
  test('commits the typed value once on Enter', async () => {
    const user = userEvent.setup()
    const onCommit = vi.fn()
    render(
      <InlineNumberChip
        label='Priority'
        value={1}
        min={-999}
        onCommit={onCommit}
      />
    )

    await user.click(screen.getByRole('button', { name: 'Priority 1' }))
    const input = screen.getByRole('textbox', { name: 'Priority' })
    await user.clear(input)
    await user.type(input, '5{Enter}')

    expect(onCommit).toHaveBeenCalledTimes(1)
    expect(onCommit).toHaveBeenCalledWith(5)
    expect(screen.queryByRole('textbox')).not.toBeInTheDocument()
  })

  test('commits when focus leaves the field', async () => {
    const user = userEvent.setup()
    const onCommit = vi.fn()
    render(
      <InlineNumberChip label='Weight' value={1} min={0} onCommit={onCommit} />
    )

    await user.click(screen.getByRole('button', { name: 'Weight 1' }))
    await user.clear(screen.getByRole('textbox', { name: 'Weight' }))
    await user.type(screen.getByRole('textbox', { name: 'Weight' }), '3')
    await user.tab()

    expect(onCommit).toHaveBeenCalledWith(3)
  })

  test('discards the edit on Escape', async () => {
    const user = userEvent.setup()
    const onCommit = vi.fn()
    render(
      <InlineNumberChip
        label='Priority'
        value={1}
        min={-999}
        onCommit={onCommit}
      />
    )

    await user.click(screen.getByRole('button', { name: 'Priority 1' }))
    await user.clear(screen.getByRole('textbox', { name: 'Priority' }))
    await user.type(
      screen.getByRole('textbox', { name: 'Priority' }),
      '9{Escape}'
    )

    expect(onCommit).not.toHaveBeenCalled()
    expect(
      screen.getByRole('button', { name: 'Priority 1' })
    ).toBeInTheDocument()
  })

  test('does not save an unchanged, empty or out-of-range value as typed', async () => {
    const user = userEvent.setup()
    const onCommit = vi.fn()
    render(
      <InlineNumberChip label='Weight' value={2} min={0} onCommit={onCommit} />
    )

    await user.click(screen.getByRole('button', { name: 'Weight 2' }))
    await user.keyboard('{Enter}')
    await user.click(screen.getByRole('button', { name: 'Weight 2' }))
    await user.clear(screen.getByRole('textbox', { name: 'Weight' }))
    await user.keyboard('{Enter}')
    expect(onCommit).not.toHaveBeenCalled()

    await user.click(screen.getByRole('button', { name: 'Weight 2' }))
    await user.clear(screen.getByRole('textbox', { name: 'Weight' }))
    await user.type(
      screen.getByRole('textbox', { name: 'Weight' }),
      '-4{Enter}'
    )
    // A weight cannot go negative: the minus sign is rejected as typed.
    expect(onCommit).toHaveBeenCalledWith(4)
  })

  test('steps with the arrow keys and clamps to the bounds', async () => {
    const user = userEvent.setup()
    const onCommit = vi.fn()
    render(
      <InlineNumberChip
        label='Cost ratio'
        value={0.02}
        formatValue={(value) => `${value}x`}
        min={0.01}
        max={1000}
        step={0.01}
        onCommit={onCommit}
      />
    )

    await user.click(screen.getByRole('button', { name: 'Cost ratio 0.02x' }))
    await user.keyboard('{ArrowDown}{ArrowDown}{ArrowDown}{Enter}')

    expect(onCommit).toHaveBeenCalledWith(0.01)
  })

  test('renders read-only when no commit handler is given', () => {
    render(<InlineNumberChip label='Cost ratio' value={0.2} min={0.01} />)

    expect(screen.queryByRole('button')).not.toBeInTheDocument()
    expect(screen.getByText('0.2')).toBeInTheDocument()
  })

  test('keeps the value box laid out and overlays the field while editing', async () => {
    const user = userEvent.setup()
    render(
      <InlineNumberChip label='Weight' value={0} min={0} onCommit={vi.fn()} />
    )

    const chip = screen.getByRole('button', { name: 'Weight 0' })
    const slot = chip.querySelector('[data-slot="aligned-number"]')
    // One-digit values still keep a roomy, aligned box.
    expect(slot?.parentElement?.style.minWidth).toBe('3ch')

    await user.click(chip)

    const input = screen.getByRole('textbox', { name: 'Weight' })
    // The field is positioned over the box inside the same chip, so it stays
    // out of the flow: the chip cannot grow or wrap while being edited.
    expect(input).toHaveClass('absolute', 'inset-0')
    expect(input.parentElement?.style.minWidth).toBe('3ch')
    expect(chip).toHaveClass('h-6', 'whitespace-nowrap')
    // Exactly one field, so the chip never gains a second control while editing.
    expect(document.body.querySelectorAll('input')).toHaveLength(1)
  })

  test('returns the focus to the chip when an edit ends with a key', async () => {
    const user = userEvent.setup()
    render(
      <InlineNumberChip
        label='Priority'
        value={1}
        min={-999}
        onCommit={vi.fn()}
      />
    )

    const chip = screen.getByRole('button', { name: 'Priority 1' })
    await user.click(chip)
    await user.keyboard('{Escape}')

    // The chip is the same element as before the edit and holds the focus, so
    // tabbing on from here continues from this cell.
    expect(screen.getByRole('button', { name: 'Priority 1' })).toBe(chip)
    expect(chip).toHaveFocus()
  })

  test('keeps the chip mounted while editing so the row cannot reflow', async () => {
    const user = userEvent.setup()
    render(
      <InlineNumberChip label='Weight' value={3} min={0} onCommit={vi.fn()} />
    )

    const chip = screen.getByRole('button', { name: 'Weight 3' })
    await user.click(chip)

    // The field lives inside the chip: no node is added beside it, so the
    // chip keeps its box and its neighbours keep theirs.
    expect(screen.getByRole('textbox', { name: 'Weight' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Weight 3' })).toBe(chip)
  })

  test('shows the bare number by default, for a column whose header names it', () => {
    render(
      <InlineNumberChip label='Weight' value={12} min={0} onCommit={vi.fn()} />
    )

    const chip = screen.getByRole('button', { name: 'Weight 12' })
    expect(chip).toHaveTextContent(/^12$/)
    // It fills the column, so the value ends on the column's right edge.
    expect(chip).toHaveClass('w-full', 'justify-end')
  })

  test('shows the label beside the number when asked, for chips side by side', () => {
    render(
      <InlineNumberChip
        label='Weight'
        labelVisible
        value={12}
        min={0}
        onCommit={vi.fn()}
      />
    )

    const chip = screen.getByRole('button', { name: 'Weight 12' })
    expect(chip).toHaveTextContent(/^Weight12$/)
    expect(chip).not.toHaveClass('w-full')
  })

  test('leaves the chip read-only when the value is a range', () => {
    render(
      <InlineNumberChip
        label='Cost ratio'
        value={null}
        displayText='0.20x-0.50x'
        min={0.01}
        onCommit={vi.fn()}
      />
    )

    expect(screen.getByText('0.20x-0.50x')).toBeVisible()
    expect(screen.queryByRole('button')).not.toBeInTheDocument()
  })
})
