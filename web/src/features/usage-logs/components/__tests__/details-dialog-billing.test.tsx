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
import { describe, expect, test, vi } from 'vitest'

import { usageLogSchema } from '../../data/schema'
import { DetailsDialog } from '../dialogs/details-dialog'

describe('details dialog billing breakdown', () => {
  test('shows H3 image charges separately from the video seconds price', () => {
    const log = usageLogSchema.parse({
      id: 2, user_id: 1, created_at: 1, type: 2, content: '',
      prompt_tokens: 0, completion_tokens: 0, quota: 712500,
      other: JSON.stringify({ model_price: 0.1125, group_ratio: 1.2,
        billing_unit: 'h3_media_v2', input_image_count: 4, input_image_price: 0.0625 }),
    })
    render(<DetailsDialog log={log} isAdmin={false} open onOpenChange={vi.fn()} />)
    expect(screen.getByText('Paid Input Images')).toBeInTheDocument()
    expect(screen.getByText('Input Image Price')).toBeInTheDocument()
    expect(screen.getByText('seconds')).toBeInTheDocument()
  })

  test('shows billing details and dynamic pricing for a tiered consume log', () => {
    const log = usageLogSchema.parse({
      id: 1,
      user_id: 1,
      created_at: 1,
      type: 2,
      content: '',
      prompt_tokens: 123,
      completion_tokens: 45,
      quota: 12500,
      other: JSON.stringify({
        model_ratio: 1,
        group_ratio: 1,
        billing_mode: 'tiered_expr',
        expr_b64: btoa('p * 1'),
      }),
    })

    render(
      <DetailsDialog log={log} isAdmin={false} open onOpenChange={vi.fn()} />
    )

    expect(screen.getByText('Billing Details')).toBeInTheDocument()
    expect(screen.getAllByText('Dynamic Pricing').length).toBeGreaterThan(0)
  })
})
