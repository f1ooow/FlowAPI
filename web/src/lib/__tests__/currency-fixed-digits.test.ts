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
import { afterEach, describe, expect, test } from 'vitest'

import {
  DEFAULT_CURRENCY_CONFIG,
  useSystemConfigStore,
} from '@/stores/system-config-store'

import { formatQuotaWithCurrency } from '../currency'

// 500_000 quota is one USD under the default currency settings.
const QUOTA_PER_USD = 500_000

function useCurrency(config: Partial<typeof DEFAULT_CURRENCY_CONFIG>): void {
  useSystemConfigStore.getState().setConfig({
    currency: { ...DEFAULT_CURRENCY_CONFIG, ...config },
  })
}

function fixed(usd: number): string {
  return formatQuotaWithCurrency(usd * QUOTA_PER_USD, {
    fixedFractionDigits: 2,
    locale: 'en-US',
  })
}

afterEach(() => useCurrency({}))

describe('formatQuotaWithCurrency fixedFractionDigits', () => {
  test('pads and rounds every amount to exactly two decimals', () => {
    expect(fixed(1.1)).toBe('$1.10')
    expect(fixed(1.57)).toBe('$1.57')
    expect(fixed(0.4715)).toBe('$0.47')
    expect(fixed(0.8282)).toBe('$0.83')
  })

  test('shows a sub-cent amount as zero instead of a minimum floor', () => {
    expect(fixed(0.0001)).toBe('$0.00')
  })

  test('drops the minus sign when a loss rounds to zero', () => {
    expect(fixed(-0.0001)).toBe('$0.00')
    expect(fixed(-1.5)).toBe('-$1.50')
  })

  test('groups thousands and never abbreviates', () => {
    expect(fixed(1_234_567.891)).toBe('$1,234,567.89')
  })

  test('applies the exchange rate before rounding for a local currency', () => {
    useCurrency({ quotaDisplayType: 'CNY', usdExchangeRate: 7 })

    expect(fixed(0.4715)).toBe('¥3.30')
  })

  test('keeps the custom symbol prefix', () => {
    useCurrency({
      quotaDisplayType: 'CUSTOM',
      customCurrencySymbol: '€',
      customCurrencyExchangeRate: 1,
    })

    expect(fixed(1.1)).toBe('€ 1.10')
  })

  test('omits the symbol when asked', () => {
    expect(
      formatQuotaWithCurrency(1.1 * QUOTA_PER_USD, {
        fixedFractionDigits: 2,
        showSymbol: false,
        locale: 'en-US',
      })
    ).toBe('1.10')
  })
})
