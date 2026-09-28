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
import { useTranslation } from 'react-i18next'

import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { toIntlLocale } from '@/i18n/languages'
import { formatQuotaWithCurrency, getCurrencyLabel } from '@/lib/currency'
import { cn } from '@/lib/utils'

import type { ChannelMonitoringTotalsWindow } from '../types'

const NO_VALUE = '--'

/**
 * The three reporting windows the board shows. They are always all visible;
 * `selectedWindow` only decides which one carries the ring and whose
 * "since when" caption is displayed.
 */
export type TotalsWindowKey = 'today' | 'month' | 'total'

const TOTALS_WINDOW_LABEL_KEY: Record<TotalsWindowKey, string> = {
  today: 'Today',
  month: 'This month',
  total: 'All time',
}

// The caption spells out the window boundary, because "today" on a page whose
// timeline is otherwise rolling 24h windows would be ambiguous.
const TOTALS_WINDOW_HINT_KEY: Record<TotalsWindowKey, string> = {
  today: 'Since midnight, UTC+8',
  month: 'Since the 1st, UTC+8',
  total: 'Since the first request',
}

/**
 * Money in the same shape the per-channel cards use: a fixed two decimals so
 * columns line up, with the precise value kept for the hover title. The one
 * deliberate difference is that negatives keep their sign, because a loss is
 * the thing this board exists to surface.
 */
function formatAmount(quota: number, locale: string | undefined) {
  const tokenSuffix = getCurrencyLabel() === 'Tokens' ? ' Tokens' : ''
  const display = `${formatQuotaWithCurrency(quota, {
    fixedFractionDigits: 2,
    locale,
  })}${tokenSuffix}`
  const title = `${formatQuotaWithCurrency(quota, {
    digitsLarge: 2,
    digitsSmall: 4,
    abbreviate: false,
    locale,
  })}${tokenSuffix}`
  return { display, title }
}

/**
 * Profit is signed and its colour carries that meaning: a positive figure is
 * revenue kept, a negative one is a window sold below cost. Muted rather than
 * saturated, matching how the rest of the page uses colour to signal state.
 */
function profitClassName(profit: number) {
  if (profit > 0) return 'text-emerald-600 dark:text-emerald-400'
  if (profit < 0) return 'text-destructive'
  return 'text-muted-foreground'
}

function TotalsCell(props: {
  label: string
  value: string
  title?: string
  valueClassName?: string
}) {
  return (
    <div className='min-w-0'>
      <div className='text-muted-foreground text-xs font-normal uppercase'>
        {props.label}
      </div>
      <div
        className={cn(
          'font-mono text-lg leading-none font-semibold tabular-nums whitespace-nowrap',
          props.valueClassName
        )}
        title={props.title}
      >
        {props.value}
      </div>
    </div>
  )
}

function TotalsWindowCard(props: {
  windowKey: TotalsWindowKey
  window: ChannelMonitoringTotalsWindow | undefined
  selected: boolean
  onSelect: (key: TotalsWindowKey) => void
}) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const label = t(TOTALS_WINDOW_LABEL_KEY[props.windowKey])

  // The window exists but its figures are unavailable (consume logging is
  // off). A dash says "not known"; zero would say "no spend", which is a
  // different and unsupported claim.
  if (!props.window) {
    return (
      <Card size='sm' className='rounded-lg'>
        <CardHeader>
          <CardTitle className='text-muted-foreground text-xs font-normal uppercase'>
            {label}
          </CardTitle>
        </CardHeader>
        <CardContent>
          <div className='text-muted-foreground font-mono text-lg leading-none font-semibold'>
            {NO_VALUE}
          </div>
        </CardContent>
      </Card>
    )
  }

  const used = formatAmount(props.window.used_quota, locale)
  const cost = formatAmount(props.window.cost_quota, locale)
  const profit = formatAmount(props.window.profit, locale)

  return (
    <Card
      size='sm'
      className={cn(
        'rounded-lg',
        props.selected && 'ring-primary/40 ring-1'
      )}
    >
      <CardHeader>
        <CardTitle className='flex items-center justify-between gap-2'>
          <button
            type='button'
            onClick={() => props.onSelect(props.windowKey)}
            aria-pressed={props.selected}
            className='text-muted-foreground hover:text-foreground text-xs font-normal uppercase transition-colors'
          >
            {label}
          </button>
          {props.window.cost_has_estimate ? (
            <span
              className='text-muted-foreground text-[10px] font-normal normal-case'
              title={t(
                'Some requests had no recorded channel cost ratio and were priced at the channel’s current one'
              )}
            >
              {t('estimated')}
            </span>
          ) : null}
        </CardTitle>
      </CardHeader>
      <CardContent>
        <div className='grid grid-cols-3 gap-3'>
          <TotalsCell
            label={t('Consumption')}
            value={used.display}
            title={used.title}
          />
          <TotalsCell label={t('Cost')} value={cost.display} title={cost.title} />
          <TotalsCell
            label={t('Profit')}
            value={profit.display}
            title={profit.title}
            valueClassName={profitClassName(props.window.profit)}
          />
        </div>
      </CardContent>
    </Card>
  )
}

/**
 * Fleet-wide consumption, cost and profit across the three reporting windows.
 *
 * The figures are summed from consume logs and priced at the cost ratio each
 * request was billed under, so they describe what happened rather than what the
 * channels are configured to cost now. Editing a channel's cost ratio, renaming
 * it, deleting it or adding one leaves these numbers alone, which is the
 * property the per-channel cards cannot provide on their own.
 */
export function ChannelMonitoringTotalsBoard(props: {
  windows: Record<TotalsWindowKey, ChannelMonitoringTotalsWindow | undefined>
  selectedWindow: TotalsWindowKey
  onSelectWindow: (key: TotalsWindowKey) => void
}) {
  const { t } = useTranslation()

  return (
    <Card size='sm' className='rounded-lg'>
      <CardHeader>
        <CardTitle className='flex flex-wrap items-baseline justify-between gap-x-3 gap-y-1'>
          <span className='text-xs font-normal uppercase'>
            {t('Fleet totals')}
          </span>
          <span className='text-muted-foreground text-[10px] font-normal normal-case'>
            {t(TOTALS_WINDOW_HINT_KEY[props.selectedWindow])}
          </span>
        </CardTitle>
      </CardHeader>
      <CardContent>
        <div className='grid gap-3 sm:grid-cols-3'>
          {(['today', 'month', 'total'] as const).map((key) => (
            <TotalsWindowCard
              key={key}
              windowKey={key}
              window={props.windows[key]}
              selected={props.selectedWindow === key}
              onSelect={props.onSelectWindow}
            />
          ))}
        </div>
      </CardContent>
    </Card>
  )
}
