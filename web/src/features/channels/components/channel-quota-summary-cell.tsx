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
import { useContext } from 'react'
import { useTranslation } from 'react-i18next'

import { textColorMap, type StatusVariant } from '@/components/status-badge'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { toIntlLocale } from '@/i18n/languages'
import { cn } from '@/lib/utils'

import {
  getChannelQuotaSummary,
  type QuotaSummaryText,
} from '../lib/channel-quota-summary'
import type { Channel, ChannelQuotaSummaryScope } from '../types'
import { ChannelRowActionsLayoutContext } from './channel-row-actions-context'
import { useChannels } from './channels-provider'

/**
 * Cost, consumption or profit (consumption minus cost) of a channel or tag over
 * the selected scope, as one column of its own. The table sizes the column and
 * right-aligns it, so the tabular figure ends on the same x on every row and
 * the decimal points line up, and the precise value stays in the tooltip. With
 * one figure per column nothing reserves room for its neighbours, so no row
 * measures the page.
 */
export function ChannelQuotaValueCell(props: {
  channel: Channel
  scope: ChannelQuotaSummaryScope
  field: 'cost' | 'used' | 'profit'
}) {
  const { t, i18n } = useTranslation()
  const layout = useContext(ChannelRowActionsLayoutContext)
  const { sensitiveVisible } = useChannels()
  const summary = getChannelQuotaSummary(props.channel, props.scope, {
    sensitiveVisible,
    showSymbol: layout !== 'card',
    locale: toIntlLocale(i18n.resolvedLanguage || i18n.language),
  })

  // Colour would give away a hidden loss, so masked values stay neutral.
  let variant: StatusVariant = 'neutral'
  if (props.field === 'profit' && sensitiveVisible) {
    if (summary.profitQuota != null && summary.profitQuota < 0) {
      variant = 'danger'
    } else if (summary.profitQuota != null && summary.profitQuota > 0) {
      variant = 'success'
    }
  }

  const entry: QuotaSummaryText = summary[props.field]
  let label = t('Profit:')
  if (props.field === 'cost') {
    label = t('Cost:')
  } else if (props.field === 'used') {
    label = t('Consumption:')
  }

  return (
    <TooltipProvider>
      <Tooltip>
        <TooltipTrigger render={<span className='inline-block cursor-help' />}>
          <span
            data-slot='aligned-number'
            title={`${label} ${entry.full}`}
            className={cn(
              'inline-block text-right font-mono font-medium tabular-nums whitespace-nowrap',
              textColorMap[variant]
            )}
          >
            {entry.display}
          </span>
        </TooltipTrigger>
        <TooltipContent>
          <p>
            {label} {entry.full}
          </p>
        </TooltipContent>
      </Tooltip>
    </TooltipProvider>
  )
}
