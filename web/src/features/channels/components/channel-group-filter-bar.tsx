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
import { useId } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'

import { useChannels } from './channels-provider'

const SENSITIVE_MASK = '••••'

/**
 * Single-select group filter shown as a row of buttons above the table, so
 * every group is one click away. `undefined` means no group filter.
 */
export function ChannelGroupFilterBar(props: {
  groups: string[]
  selectedGroup: string | undefined
  onSelectedGroupChange: (group: string | undefined) => void
}) {
  const { t } = useTranslation()
  const { sensitiveVisible } = useChannels()
  const labelId = useId()
  const isAllSelected = props.selectedGroup === undefined

  return (
    <div
      role='group'
      aria-labelledby={labelId}
      className='flex min-w-0 flex-1 flex-wrap items-center gap-1'
    >
      <span id={labelId} className='text-muted-foreground me-1 text-sm'>
        {t('Group:')}
      </span>
      <Button
        size='sm'
        variant={isAllSelected ? 'default' : 'ghost'}
        aria-pressed={isAllSelected}
        onClick={() => props.onSelectedGroupChange(undefined)}
      >
        {t('All')}
      </Button>
      {props.groups.map((group) => (
        <Button
          key={group}
          size='sm'
          variant={group === props.selectedGroup ? 'default' : 'ghost'}
          aria-pressed={group === props.selectedGroup}
          onClick={() => props.onSelectedGroupChange(group)}
        >
          {sensitiveVisible ? group : SENSITIVE_MASK}
        </Button>
      ))}
    </div>
  )
}
