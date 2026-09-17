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

function renderDetails(settlementStatus?: string) {
  const log = usageLogSchema.parse({
    id: 1,
    user_id: 1,
    created_at: 1,
    type: 2,
    content: '',
    prompt_tokens: 123,
    completion_tokens: 45,
    quota: settlementStatus === 'settled' ? 12500 : 0,
    other: JSON.stringify({
      model_ratio: 1,
      group_ratio: 1,
      billing_mode: 'tiered_expr',
      expr_b64: btoa('p * 1'),
      hedge: settlementStatus
        ? {
            attempt_id: 'request-1:2',
            attempt: 2,
            role: 'loser',
            usage_source: 'upstream',
            metering_status: 'complete',
            settlement_status: settlementStatus,
          }
        : undefined,
    }),
  })
  return render(
    <DetailsDialog log={log} isAdmin={false} open onOpenChange={vi.fn()} />
  )
}

describe('hedge billing details', () => {
  test.each(['not_billed', 'failed', 'unmetered', 'future_status'])(
    'hides billed costs for %s while retaining known tokens and loser identity',
    (status) => {
      renderDetails(status)
      expect(screen.getByText('Extra racing attempt')).toBeInTheDocument()
      expect(screen.getByText('Token Breakdown')).toBeInTheDocument()
      expect(screen.getByText('123')).toBeInTheDocument()
      expect(screen.getByText('45')).toBeInTheDocument()
      expect(screen.queryByText('Billing Details')).not.toBeInTheDocument()
      expect(screen.queryByText('Dynamic Pricing')).not.toBeInTheDocument()
    }
  )

  test.each(['settled', undefined])(
    'preserves billing breakdowns for %s settlement',
    (status) => {
      renderDetails(status)
      expect(screen.getByText('Billing Details')).toBeInTheDocument()
      expect(screen.getAllByText('Dynamic Pricing').length).toBeGreaterThan(0)
    }
  )
})
