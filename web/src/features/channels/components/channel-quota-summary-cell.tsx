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

import { textColorMap, type StatusVariant } from '@/components/status-badge'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { toIntlLocale } from '@/i18n/languages'

import {
  getChannelQuotaSummary,
  type QuotaSummaryText,
} from '../lib/channel-quota-summary'
import type { Channel, ChannelQuotaSummaryScope } from '../types'
import { AlignedValue } from './aligned-value'
import { ChannelColumnSlotsContext } from './channel-column-slots-context'
import { ChannelRowActionsLayoutContext } from './channel-row-actions-context'
import { useChannels } from './channels-provider'

type QuotaSummaryEntry = QuotaSummaryText & {
  key: 'cost' | 'used' | 'profit'
  label: string
  variant: StatusVariant
  width: number | undefined
}

/**
 * Cost, consumption and profit (consumption minus cost) of a channel or tag
 * over the selected scope. In the table each figure sits in a right-aligned
 * slot whose width the table shares across its rows, so the amounts and the
 * separators line up down the column.
 */
export function ChannelQuotaSummaryCell(props: {
  channel: Channel
  scope: ChannelQuotaSummaryScope
}) {
  const { t, i18n } = useTranslation()
  const layout = useContext(ChannelRowActionsLayoutContext)
  const slotWidths = useContext(ChannelColumnSlotsContext)
  const { sensitiveVisible } = useChannels()
  const summary = getChannelQuotaSummary(props.channel, props.scope, {
    sensitiveVisible,
    showSymbol: layout !== 'card',
    locale: toIntlLocale(i18n.resolvedLanguage || i18n.language),
  })

  // Colour would give away a hidden loss, so masked values stay neutral.
  let profitVariant: StatusVariant = 'neutral'
  if (sensitiveVisible && summary.profitQuota != null) {
    if (summary.profitQuota < 0) {
      profitVariant = 'danger'
    } else if (summary.profitQuota > 0) {
      profitVariant = 'success'
    }
  }

  // Cards are laid out one by one, so only the table aligns its slots.
  const alignedWidths = layout === 'table' ? slotWidths : null
  const entries: QuotaSummaryEntry[] = [
    {
      key: 'cost',
      label: t('Cost:'),
      variant: 'neutral',
      width: alignedWidths?.cost,
      ...summary.cost,
    },
    {
      key: 'used',
      label: t('Consumption:'),
      variant: 'neutral',
      width: alignedWidths?.used,
      ...summary.used,
    },
    {
      key: 'profit',
      label: t('Profit:'),
      variant: profitVariant,
      width: alignedWidths?.profit,
      ...summary.profit,
    },
  ]

  return (
    <TooltipProvider>
      <Tooltip>
        <TooltipTrigger
          render={
            <div className='flex max-w-full min-w-0 cursor-help items-center gap-1 text-sm font-medium' />
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
              <AlignedValue
                width={entry.width}
                className={textColorMap[entry.variant]}
                title={`${entry.label} ${entry.full}`}
              >
                {entry.display}
              </AlignedValue>
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
