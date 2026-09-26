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
import { formatQuotaWithCurrency, getCurrencyLabel } from '@/lib/currency'

import type { Channel, ChannelQuotaSummaryScope } from '../types'

const SENSITIVE_MASK = '••••'
const NO_VALUE = '--'

export type QuotaSummaryText = {
  /** Inline text, always with two decimals so the figures line up. */
  display: string
  /** Precise value (up to four decimals) for the tooltip. */
  full: string
}

export type ChannelQuotaSummary = {
  cost: QuotaSummaryText
  used: QuotaSummaryText
  profit: QuotaSummaryText
  /** Consumption minus cost; only known when both sides are. */
  profitQuota: number | null
}

export type QuotaSummaryFormat = {
  sensitiveVisible: boolean
  showSymbol: boolean
  locale: string | undefined
}

/**
 * Unavailable figures stay `--` even when values are hidden, since there is
 * nothing to mask.
 */
function formatQuotaSummaryText(
  quota: number | null | undefined,
  format: QuotaSummaryFormat,
  tokenSuffix: string
): QuotaSummaryText {
  if (quota == null) {
    return { display: NO_VALUE, full: NO_VALUE }
  }
  if (!format.sensitiveVisible) {
    return { display: SENSITIVE_MASK, full: SENSITIVE_MASK }
  }
  const display = `${formatQuotaWithCurrency(quota, {
    fixedFractionDigits: 2,
    showSymbol: format.showSymbol,
    locale: format.locale,
  })}${tokenSuffix}`
  const full = `${formatQuotaWithCurrency(quota, {
    digitsLarge: 2,
    digitsSmall: 4,
    abbreviate: false,
    showSymbol: format.showSymbol,
    locale: format.locale,
  })}${tokenSuffix}`
  return { display, full }
}

/**
 * Cost, consumption and profit of a channel or tag over a scope, as the texts
 * the table shows. The cell renders these texts and the table sizes its
 * aligned columns from them, so both always agree.
 */
export function getChannelQuotaSummary(
  channel: Channel,
  scope: ChannelQuotaSummaryScope,
  format: QuotaSummaryFormat
): ChannelQuotaSummary {
  const isToday = scope === 'today'
  const costQuota = isToday
    ? channel.today_cost_quota
    : channel.total_cost_quota
  const usedQuota = isToday
    ? channel.today_used_quota
    : channel.total_used_quota
  const profitQuota =
    costQuota != null && usedQuota != null ? usedQuota - costQuota : null
  const tokenSuffix = getCurrencyLabel() === 'Tokens' ? ' Tokens' : ''
  return {
    cost: formatQuotaSummaryText(costQuota, format, tokenSuffix),
    used: formatQuotaSummaryText(usedQuota, format, tokenSuffix),
    profit: formatQuotaSummaryText(profitQuota, format, tokenSuffix),
    profitQuota,
  }
}
