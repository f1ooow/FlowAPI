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
import { flexRender } from '@tanstack/react-table'
import { render, screen } from '@testing-library/react'
import { describe, expect, test, vi } from 'vitest'

import { useUpstreamRatioSyncColumns } from '../upstream-ratio-sync-columns'
import type { ModelRow } from '../upstream-ratio-sync-helpers'

const translate = (key: string) => key
vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: translate }),
}))

const UPSTREAM = 'Acme upstream'

const row: ModelRow = {
  key: 'gpt-4o',
  model: 'gpt-4o',
  ratioTypes: {
    model_ratio: {
      current: 1,
      upstreams: { [UPSTREAM]: 2.5 },
      confidence: { [UPSTREAM]: true },
    },
    completion_ratio: {
      current: 3,
      upstreams: { [UPSTREAM]: 4 },
      confidence: { [UPSTREAM]: true },
    },
  },
  billingConflict: false,
}

/**
 * Renders only the upstream value column body. Mounting the full sync table
 * would drag in the settings page shell without exercising anything extra for
 * the accessible-name contract under test.
 */
function UpstreamCellHarness() {
  const columns = useUpstreamRatioSyncColumns(
    [UPSTREAM],
    {},
    {},
    '__all__',
    false,
    () => {},
    () => {},
    () => {},
    () => {}
  )
  const column = columns.find((c) => c.id === `upstream_${UPSTREAM}`)
  const cell = column?.cell

  if (typeof cell !== 'function') return null

  return (
    <div>
      {flexRender(cell, {
        row: { original: row },
      } as never)}
    </div>
  )
}

describe('upstream ratio sync value checkboxes', () => {
  test('exposes an accessible name combining model, ratio type and value', () => {
    render(<UpstreamCellHarness />)

    expect(
      screen.getByRole('checkbox', { name: 'gpt-4o Model ratio 2.5' })
    ).toBeInTheDocument()
    expect(
      screen.getByRole('checkbox', { name: 'gpt-4o Completion ratio 4' })
    ).toBeInTheDocument()
  })

  test('gives every value checkbox a distinct accessible name', () => {
    render(<UpstreamCellHarness />)

    const names = screen
      .getAllByRole('checkbox')
      .map((checkbox) => checkbox.getAttribute('aria-label'))

    expect(names).toHaveLength(2)
    expect(names.every((name) => Boolean(name))).toBe(true)
    expect(new Set(names).size).toBe(names.length)
  })
})
