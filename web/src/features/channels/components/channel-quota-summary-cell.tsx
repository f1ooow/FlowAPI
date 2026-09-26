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
import { Fragment, useContext } from 'react'
import { useTranslation } from 'react-i18next'

import { StatusBadge, type StatusVariant } from '@/components/status-badge'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { toIntlLocale } from '@/i18n/languages'
import { formatQuotaWithCurrency, getCurrencyLabel } from '@/lib/currency'

import type { Channel, ChannelQuotaSummaryScope } from '../types'
import { ChannelRowActionsLayoutContext } from './channel-row-actions-context'
import { useChannels } from './channels-provider'

const MAX_INLINE_QUOTA_CHARS = 8
const SENSITIVE_MASK = '••••'
const NO_VALUE = '--'

type QuotaFormatOptions = {
  sensitiveVisible: boolean
  showSymbol: boolean
  locale: string | undefined
  tokenSuffix: string
}

type QuotaSummaryEntry = {
  key: 'cost' | 'used' | 'profit'
  label: string
  variant: StatusVariant
  display: string
  full: string
}

/**
 * Inline text plus the precise value for the tooltip. Unavailable figures stay
 * `--` even when values are hidden, since there is nothing to mask.
 */
function formatSummaryQuota(
  quota: number | null | undefined,
  options: QuotaFormatOptions
): { display: string; full: string } {
  if (quota == null) {
    return { display: NO_VALUE, full: NO_VALUE }
  }
  if (!options.sensitiveVisible) {
    return { display: SENSITIVE_MASK, full: SENSITIVE_MASK }
  }

  const full = `${formatQuotaWithCurrency(quota, {
    digitsLarge: 2,
    digitsSmall: 4,
    abbreviate: false,
    showSymbol: options.showSymbol,
    locale: options.locale,
  })}${options.tokenSuffix}`
  if (full.length <= MAX_INLINE_QUOTA_CHARS) {
    return { display: full, full }
  }

  const compact = formatQuotaWithCurrency(quota, {
    compact: true,
    showSymbol: options.showSymbol,
    locale: options.locale,
  })
  return { display: `${compact}${options.tokenSuffix}`, full }
}

/**
 * Cost, consumption and profit (consumption minus cost) of a channel or tag
 * over the selected scope. Profit is only known when both sides are.
 */
export function ChannelQuotaSummaryCell(props: {
  channel: Channel
  scope: ChannelQuotaSummaryScope
}) {
  const { t, i18n } = useTranslation()
  const layout = useContext(ChannelRowActionsLayoutContext)
  const { sensitiveVisible } = useChannels()
  const isToday = props.scope === 'today'
  const costQuota = isToday
    ? props.channel.today_cost_quota
    : props.channel.total_cost_quota
  const usedQuota = isToday
    ? props.channel.today_used_quota
    : props.channel.total_used_quota
  const profitQuota =
    costQuota != null && usedQuota != null ? usedQuota - costQuota : null

  const formatOptions: QuotaFormatOptions = {
    sensitiveVisible,
    showSymbol: layout !== 'card',
    locale: toIntlLocale(i18n.resolvedLanguage || i18n.language),
    tokenSuffix: getCurrencyLabel() === 'Tokens' ? ' Tokens' : '',
  }

  // Colour would give away a hidden loss, so masked values stay neutral.
  let profitVariant: StatusVariant = 'neutral'
  if (sensitiveVisible && profitQuota != null && profitQuota < 0) {
    profitVariant = 'danger'
  } else if (sensitiveVisible && profitQuota != null && profitQuota > 0) {
    profitVariant = 'success'
  }

  const entries: QuotaSummaryEntry[] = [
    {
      key: 'cost',
      label: t('Cost:'),
      variant: 'neutral',
      ...formatSummaryQuota(costQuota, formatOptions),
    },
    {
      key: 'used',
      label: t('Consumption:'),
      variant: 'neutral',
      ...formatSummaryQuota(usedQuota, formatOptions),
    },
    {
      key: 'profit',
      label: t('Profit:'),
      variant: profitVariant,
      ...formatSummaryQuota(profitQuota, formatOptions),
    },
  ]

  return (
    <TooltipProvider>
      <Tooltip>
        <TooltipTrigger
          render={
            <div className='-ml-1.5 flex max-w-full min-w-0 cursor-help items-center' />
          }
        >
          {entries.map((entry, index) => (
            <Fragment key={entry.key}>
              {index > 0 && (
                <span
                  aria-hidden='true'
                  className='text-muted-foreground/60 shrink-0 text-xs'
                >
                  /
                </span>
              )}
              <StatusBadge
                label={entry.display}
                variant={entry.variant}
                size='sm'
                copyable={false}
                title={`${entry.label} ${entry.full}`}
                className='tabular-nums'
              />
            </Fragment>
          ))}
        </TooltipTrigger>
        <TooltipContent>
          <div className='grid gap-0.5'>
            {entries.map((entry) => (
              <p key={entry.key}>
                {entry.label} {entry.full}
              </p>
            ))}
          </div>
        </TooltipContent>
      </Tooltip>
    </TooltipProvider>
  )
}
