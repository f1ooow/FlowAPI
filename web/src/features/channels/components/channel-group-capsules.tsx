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
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Plus } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { GroupBadge } from '@/components/group-badge'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'

import { getGroups } from '../api'
import { handleUpdateChannelGroups, parseGroupsList } from '../lib'
import type { Channel } from '../types'
import { useChannels } from './channels-provider'

const SENSITIVE_MASK = '••••'

/**
 * Group capsules of a channel. Clicking them opens a searchable checkbox grid
 * of every group; each tick saves the channel's groups right away. A channel
 * without groups shows an "Add group" button that opens the same grid.
 */
export function ChannelGroupCapsules(props: { channel: Channel }) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const { sensitiveVisible } = useChannels()
  const [open, setOpen] = useState(false)
  const [search, setSearch] = useState('')
  const [isSaving, setIsSaving] = useState(false)
  // Same query key as the table's group filter, so both share one request.
  const { data: groupsData } = useQuery({
    queryKey: ['groups'],
    queryFn: getGroups,
    enabled: open,
  })
  const groups = parseGroupsList(props.channel.group)

  // Groups on the channel that the group list no longer knows stay visible,
  // so they can still be unticked.
  const availableGroups = groupsData?.data ?? []
  const allGroups = [
    ...availableGroups,
    ...groups.filter((group) => !availableGroups.includes(group)),
  ]
  const keyword = search.trim().toLocaleLowerCase()
  const visibleGroups = keyword
    ? allGroups.filter((group) => group.toLocaleLowerCase().includes(keyword))
    : allGroups

  const toggleGroup = async (group: string, checked: boolean) => {
    const next = checked
      ? [...groups, group]
      : groups.filter((current) => current !== group)
    if (next.length === 0) {
      toast.error(t('A channel needs at least one group'))
      return
    }
    setIsSaving(true)
    try {
      await handleUpdateChannelGroups(props.channel.id, next, queryClient)
    } finally {
      setIsSaving(false)
    }
  }

  const trigger =
    groups.length > 0 ? (
      <PopoverTrigger
        aria-label={t('Edit groups')}
        className='-ml-1.5 flex max-w-full min-w-0 flex-wrap items-center gap-1 rounded-md text-left'
      >
        {groups.map((group) => (
          <GroupBadge
            key={group}
            group={group}
            label={sensitiveVisible ? undefined : SENSITIVE_MASK}
            size='sm'
            className='cursor-pointer'
          />
        ))}
      </PopoverTrigger>
    ) : (
      <PopoverTrigger className='text-muted-foreground hover:text-foreground hover:bg-muted inline-flex h-5 items-center gap-0.5 rounded-md border border-dashed px-1.5 text-xs'>
        <Plus className='size-3' aria-hidden='true' />
        {t('Add group')}
      </PopoverTrigger>
    )

  return (
    <Popover
      open={open}
      onOpenChange={(nextOpen) => {
        setOpen(nextOpen)
        if (!nextOpen) {
          setSearch('')
        }
      }}
    >
      {trigger}
      <PopoverContent align='start' className='w-80 gap-2 p-2'>
        <Input
          value={search}
          onChange={(event) => setSearch(event.target.value)}
          placeholder={t('Search groups...')}
          aria-label={t('Search groups...')}
          className='h-8'
        />
        {visibleGroups.length === 0 ? (
          <p className='text-muted-foreground px-1 py-2 text-center text-xs'>
            {t('No results found.')}
          </p>
        ) : (
          <div className='grid max-h-60 grid-cols-3 gap-1 overflow-y-auto'>
            {visibleGroups.map((group) => {
              const checked = groups.includes(group)
              const label = sensitiveVisible ? group : SENSITIVE_MASK
              // Not wrapped in a <label>: Base UI would link it through
              // aria-labelledby and read the name twice. The text only
              // toggles on its own clicks, so one click is one change.
              return (
                <div
                  key={group}
                  title={label}
                  className='hover:bg-muted flex min-w-0 items-center gap-1.5 rounded-md px-1.5 py-1 text-xs'
                >
                  <Checkbox
                    checked={checked}
                    disabled={isSaving}
                    onCheckedChange={(nextChecked) =>
                      void toggleGroup(group, nextChecked)
                    }
                    aria-label={label}
                  />
                  <span
                    aria-hidden='true'
                    onClick={() => {
                      if (!isSaving) {
                        void toggleGroup(group, !checked)
                      }
                    }}
                    className='min-w-0 flex-1 cursor-pointer truncate'
                  >
                    {label}
                  </span>
                </div>
              )
            })}
          </div>
        )}
      </PopoverContent>
    </Popover>
  )
}
