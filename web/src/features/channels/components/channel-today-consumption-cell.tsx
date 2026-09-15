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

import { StatusBadge } from '@/components/status-badge'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { toIntlLocale } from '@/i18n/languages'
import { formatQuotaWithCurrency, getCurrencyLabel } from '@/lib/currency'

import type { Channel } from '../types'
import { ChannelRowActionsLayoutContext } from './channel-row-actions-context'
import { useChannels } from './channels-provider'

const MAX_INLINE_QUOTA_CHARS = 8
const SENSITIVE_MASK = '••••'

export function ChannelTodayConsumptionCell(props: { channel: Channel }) {
  const { i18n } = useTranslation()
  const layout = useContext(ChannelRowActionsLayoutContext)
  const { sensitiveVisible } = useChannels()
  const quota = props.channel.today_used_quota

  if (quota == null) {
    return <span className='text-muted-foreground text-xs'>--</span>
  }

  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const showSymbol = layout !== 'card'
  const tokenSuffix = getCurrencyLabel() === 'Tokens' ? ' Tokens' : ''
  const fullValueBase = formatQuotaWithCurrency(quota, {
    digitsLarge: 2,
    digitsSmall: 4,
    abbreviate: false,
    showSymbol,
    locale,
  })
  const fullValue = `${fullValueBase}${tokenSuffix}`

  let displayValue = fullValue
  if (fullValue.length > MAX_INLINE_QUOTA_CHARS) {
    const compactValue = formatQuotaWithCurrency(quota, {
      compact: true,
      showSymbol,
      locale,
    })
    displayValue = `${compactValue}${tokenSuffix}`
  }

  const visibleValue = sensitiveVisible ? displayValue : SENSITIVE_MASK
  const tooltipValue = sensitiveVisible ? fullValue : SENSITIVE_MASK

  return (
    <TooltipProvider>
      <Tooltip>
        <TooltipTrigger
          render={
            <StatusBadge
              label={visibleValue}
              variant='neutral'
              size='sm'
              copyable={false}
              showDot={false}
              title={tooltipValue}
              className='-ml-1.5 cursor-help'
            />
          }
        />
        <TooltipContent>
          <p>{tooltipValue}</p>
        </TooltipContent>
      </Tooltip>
    </TooltipProvider>
  )
}
