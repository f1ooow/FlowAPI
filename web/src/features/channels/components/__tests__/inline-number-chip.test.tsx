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
})
