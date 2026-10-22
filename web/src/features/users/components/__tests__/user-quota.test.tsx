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
import { afterEach, beforeEach, describe, expect, test } from 'vitest'

import { TooltipProvider } from '@/components/ui/tooltip'
import {
  DEFAULT_CURRENCY_CONFIG,
  useSystemConfigStore,
} from '@/stores/system-config-store'

import { UserQuotaCell } from '../user-quota-cell'

beforeEach(() => {
  useSystemConfigStore.getState().setConfig({
    currency: DEFAULT_CURRENCY_CONFIG,
  })
})

afterEach(() => {
  useSystemConfigStore.getState().setConfig({
    currency: DEFAULT_CURRENCY_CONFIG,
  })
})

describe('user quota display', () => {
  test('shows actual used quota without opening a tooltip and preserves finite remaining, total, and progress', () => {
    render(
      <TooltipProvider>
        <UserQuotaCell used={250_000} remaining={1_000_000} unlimited={false} />
      </TooltipProvider>
    )

    expect(screen.getByText('Used: $0.5')).toBeVisible()
    expect(screen.getByText('$2')).toBeVisible()
    expect(screen.getByText('$2.5')).toBeVisible()
    expect(screen.getByRole('progressbar')).toHaveAttribute(
      'aria-valuenow',
      '80'
    )
  })

  test('shows zero usage for an unused finite quota instead of hiding the used amount', () => {
    render(
      <TooltipProvider>
        <UserQuotaCell used={0} remaining={500_000} unlimited={false} />
      </TooltipProvider>
    )

    expect(screen.getByText('Used: $0')).toBeVisible()
    expect(screen.getByRole('progressbar')).toHaveAttribute(
      'aria-valuenow',
      '100'
    )
  })

  test('preserves unlimited quota and its visible used amount without adding finite progress', () => {
    render(
      <TooltipProvider>
        <UserQuotaCell used={250_000} remaining={0} unlimited />
      </TooltipProvider>
    )

    expect(screen.getByText('∞')).toBeVisible()
    expect(screen.getByText('Used: $0.5')).toBeVisible()
    expect(screen.queryByRole('progressbar')).not.toBeInTheDocument()
  })

  test('preserves the no-quota state when remaining and used amounts are both zero', () => {
    render(
      <TooltipProvider>
        <UserQuotaCell used={0} remaining={0} unlimited={false} />
      </TooltipProvider>
    )

    expect(screen.getByText('No Quota')).toBeVisible()
    expect(screen.queryByRole('progressbar')).not.toBeInTheDocument()
    expect(screen.queryByText(/^Used:/)).not.toBeInTheDocument()
  })
})
