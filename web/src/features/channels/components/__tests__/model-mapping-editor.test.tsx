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

import { parseModelRedirectRules } from '../../lib/model-redirect-rules'
import { ModelMappingEditor } from '../model-mapping-editor'

const RULES = JSON.stringify([
  { match_type: 'contains', source: 'claude', target: 'generic-claude' },
  { match_type: 'contains', source: 'opus', target: 'claude-opus-4-6' },
])

describe('ModelMappingEditor', () => {
  test('shows the first ordered match when testing a requested model', async () => {
    const user = userEvent.setup()
    render(<ModelMappingEditor value={RULES} onChange={() => undefined} />)

    await user.type(
      screen.getByRole('textbox', { name: 'Model name to test' }),
      'claude-opus-5'
    )
    await user.click(screen.getByRole('button', { name: 'Test rules' }))

    expect(screen.getByText('generic-claude')).toBeVisible()
    expect(screen.getByText(/Rule 1 matched/)).toBeVisible()
  })

  test('reorders rules and persists their new array order', async () => {
    const onChange = vi.fn()
    const user = userEvent.setup()
    render(<ModelMappingEditor value={RULES} onChange={onChange} />)

    const moveUpButtons = screen.getAllByRole('button', {
      name: 'Move rule up',
    })
    await user.click(moveUpButtons[1])

    const latestValue = String(onChange.mock.calls.at(-1)?.[0])
    expect(parseModelRedirectRules(latestValue)[0].target).toBe(
      'claude-opus-4-6'
    )
  })
})
