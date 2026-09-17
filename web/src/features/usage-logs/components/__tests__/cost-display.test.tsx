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
import i18next from 'i18next'
import type React from 'react'
import { beforeAll, describe, expect, test } from 'vitest'

import { formatLogQuota } from '@/lib/format'

import type { LogOtherData } from '../../types'
import { HedgeAttemptStatus } from '../hedge-attempt-status'
import { LogCostDisplay } from '../log-cost-display'

function renderCost(
  props: React.ComponentProps<typeof LogCostDisplay>
): ReturnType<typeof render> {
  return render(<LogCostDisplay {...props} />)
}

function normalizedText(value: string | null): string {
  return (value ?? '').replaceAll(/\s/g, '')
}

describe('log cost display', () => {
  const losingAttempt: NonNullable<LogOtherData['hedge']> = {
    attempt_id: 'request-1:2',
    attempt: 2,
    role: 'loser',
    usage_source: 'upstream',
    metering_status: 'complete',
    settlement_status: 'settled',
  }

  test('identifies a charged loser as an extra attempt and retains its separate charge', () => {
    const rendered = render(
      <>
        <HedgeAttemptStatus hedge={losingAttempt} />
        <LogCostDisplay quota={12500} other={{ hedge: losingAttempt }} />
      </>
    )
    expect(screen.getByText('Extra racing attempt')).toBeInTheDocument()
    expect(screen.getByText('Metered usage')).toBeInTheDocument()
    expect(screen.queryByText('Success')).not.toBeInTheDocument()
    expect(normalizedText(rendered.container.textContent)).toContain(
      normalizedText(formatLogQuota(12500))
    )
  })

  test('shows missing metering as unknown and uncharged without implying a zero upstream cost', () => {
    const rendered = renderCost({
      quota: 0,
      other: {
        hedge: {
          ...losingAttempt,
          usage_source: 'unknown',
          metering_status: 'unmetered',
          settlement_status: 'unmetered',
        },
      },
    })
    expect(screen.getByText('Usage unknown')).toBeInTheDocument()
    expect(screen.getByText('Not charged')).toBeInTheDocument()
    expect(normalizedText(rendered.container.textContent)).not.toContain(
      normalizedText(formatLogQuota(0))
    )
  })

  test.each([
    ['complete', 'Metered usage'],
    ['partial', 'Partial usage'],
  ] as const)(
    'keeps %s metering and loser identity when policy excludes billing',
    (metering_status, label) => {
      const hedge = {
        ...losingAttempt,
        metering_status,
        settlement_status: 'not_billed' as const,
      }
      const rendered = render(
        <>
          <HedgeAttemptStatus hedge={hedge} />
          <LogCostDisplay quota={0} other={{ hedge }} />
        </>
      )
      expect(screen.getByText('Extra racing attempt')).toBeInTheDocument()
      expect(screen.getByText('Not billed')).toBeInTheDocument()
      expect(screen.getByText(label)).toBeInTheDocument()
      expect(screen.queryByText('Usage unknown')).not.toBeInTheDocument()
      expect(normalizedText(rendered.container.textContent)).not.toContain(
        normalizedText(formatLogQuota(0))
      )
    }
  )

  test('shows failed settlement separately from a successful or zero charge', () => {
    renderCost({
      quota: 12500,
      other: { hedge: { ...losingAttempt, settlement_status: 'failed' } },
    })
    expect(screen.getByText('Settlement failed')).toBeInTheDocument()
    expect(screen.queryByText('Not charged')).not.toBeInTheDocument()
  })

  test.each([
    ['upstream', 'partial', 'Partial usage'],
    ['estimated', 'complete', 'Estimated usage'],
  ] as const)(
    'distinguishes %s/%s metering from complete upstream usage',
    (usage_source, metering_status, label) => {
      renderCost({
        quota: 12500,
        other: { hedge: { ...losingAttempt, usage_source, metering_status } },
      })
      expect(screen.getByText(label)).toBeInTheDocument()
      expect(screen.queryByText('Metered usage')).not.toBeInTheDocument()
    }
  )

  beforeAll(() => {
    i18next.addResourceBundle('en', 'translation', {
      Subscription: 'Subscription',
      'Deducted by subscription': 'Deducted by subscription',
      'Includes tool-call surcharge': 'Includes tool-call surcharge',
    })
  })

  test('keeps the regular cost visible and adds an accessible surcharge marker', () => {
    const rendered = renderCost({
      quota: 12500,
      other: {
        tool_surcharges: [{ name: 'lookup_customer', count: 1, price: 5 }],
      },
    })

    expect(
      normalizedText(rendered.container.textContent).includes(
        normalizedText(formatLogQuota(12500))
      )
    ).toBe(true)
    const marker = screen.getByRole('img', {
      name: 'Includes tool-call surcharge',
    })
    expect(marker).toHaveAttribute('data-tool-surcharge-indicator', 'true')
    expect(marker).toHaveAttribute('tabindex', '0')
  })

  test('preserves the subscription badge and adds the same legacy surcharge marker', () => {
    renderCost({
      quota: 5000,
      other: {
        billing_source: 'subscription',
        web_search: true,
        web_search_call_count: 1,
        web_search_price: 10,
      },
    })

    expect(screen.getByText('Subscription')).toBeInTheDocument()
    expect(
      screen.getByRole('img', { name: 'Includes tool-call surcharge' })
    ).toHaveAttribute('data-tool-surcharge-indicator', 'true')
  })
})
