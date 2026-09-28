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
import { ChevronDown } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuLabel,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'

import { getChannelQuotaColumnTitle } from '../lib'
import type { ChannelQuotaSummaryScope } from '../types'

/**
 * Header of the cost column, the first of the cost / consumption / profit
 * columns. The figures come from consume logs and cannot be sorted
 * server-side, so the menu offers the statistics window of all three columns
 * instead of sort directions. It is shaped like the table's sort header (title,
 * then icon), so the numeric column header class aligns both the same way.
 */
export function ChannelQuotaSummaryHeader(props: {
  scope: ChannelQuotaSummaryScope
  onScopeChange: (scope: ChannelQuotaSummaryScope) => void
}) {
  const { t } = useTranslation()

  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        render={
          <Button
            variant='ghost'
            size='sm'
            className='data-popup-open:bg-accent -ms-3 h-8'
          />
        }
      >
        <span>{getChannelQuotaColumnTitle('cost', props.scope, t)}</span>
        <ChevronDown className='ms-2 h-4 w-4' />
      </DropdownMenuTrigger>
      <DropdownMenuContent align='end'>
        <DropdownMenuRadioGroup
          value={props.scope}
          onValueChange={(value) =>
            props.onScopeChange(value === 'total' ? 'total' : 'today')
          }
        >
          <DropdownMenuLabel>{t('Statistics range')}</DropdownMenuLabel>
          <DropdownMenuRadioItem value='today' closeOnClick>
            {t('Today so far')}
          </DropdownMenuRadioItem>
          <DropdownMenuRadioItem value='total' closeOnClick>
            {t('All time')}
          </DropdownMenuRadioItem>
        </DropdownMenuRadioGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
