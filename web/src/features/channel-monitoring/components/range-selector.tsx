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

import { Button } from '@/components/ui/button'

/**
 * The available ranges come from the API response, so the switcher never
 * offers a value the backend would reject. The tokens (`15m`, `24h`, `7d`) are
 * rendered verbatim on purpose: they are the same in every language.
 */
export function RangeSelector(props: {
  ranges: string[]
  value: string
  onChange: (range: string) => void
}) {
  const { t } = useTranslation()

  return (
    <div
      role='group'
      aria-label={t('Time range')}
      className='flex flex-wrap items-center gap-1'
    >
      {props.ranges.map((range) => (
        <Button
          key={range}
          size='sm'
          variant={range === props.value ? 'default' : 'ghost'}
          aria-pressed={range === props.value}
          onClick={() => props.onChange(range)}
        >
          {range}
        </Button>
      ))}
    </div>
  )
}
