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
import { useQueryClient } from '@tanstack/react-query'
import { Plus } from 'lucide-react'
import { useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import { cn } from '@/lib/utils'

import {
  useChannelGroups,
  useGroupCapsuleClassName,
} from '../hooks/use-channel-groups'
import { handleUpdateChannelGroups, parseGroupsList } from '../lib'
import type { Channel } from '../types'
import { useChannels } from './channels-provider'

const SENSITIVE_MASK = '••••'

/**
 * A solid, white-on-colour pill. The fill comes from the full group list, so
 * a group has the same colour in every row, card and popover.
 */
export function GroupCapsule(props: { group: string; className?: string }) {
  const { sensitiveVisible } = useChannels()
  const capsuleClassName = useGroupCapsuleClassName()
  return (
    <span
      data-slot='group-capsule'
      className={cn(
        'inline-flex max-w-full items-center rounded-md px-2 py-0.5 text-xs leading-4 font-medium text-white',
        capsuleClassName(props.group),
        props.className
      )}
    >
      {sensitiveVisible ? props.group : SENSITIVE_MASK}
    </span>
  )
}

/**
 * The groups of a channel as capsules that sit inline after its name. Any
 * capsule opens a searchable checkbox grid of every group; each tick saves the
 * channel's groups right away. A channel without groups shows an outlined
 * "Add group" button that opens the same grid.
 *
 * Renders a fragment so the capsules join the parent's wrapping name row.
 */
export function ChannelGroupCapsules(props: { channel: Channel }) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const { sensitiveVisible } = useChannels()
  const [open, setOpen] = useState(false)
  const [search, setSearch] = useState('')
  const [isSaving, setIsSaving] = useState(false)
  // Unticking a capsule's own group removes that capsule, so the popup is
  // anchored to the first one instead of whichever trigger opened it.
  const anchorRef = useRef<HTMLButtonElement>(null)
  const { data: groupsData } = useChannelGroups()
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
      {groups.length > 0 ? (
        groups.map((group, index) => (
          <PopoverTrigger
            key={group}
            ref={index === 0 ? anchorRef : undefined}
            aria-label={t('Edit groups: {{group}}', {
              group: sensitiveVisible ? group : SENSITIVE_MASK,
            })}
            className='focus-visible:ring-ring/50 max-w-full shrink-0 cursor-pointer rounded-md outline-none focus-visible:ring-2'
          >
            <GroupCapsule group={group} />
          </PopoverTrigger>
        ))
      ) : (
        <PopoverTrigger
          ref={anchorRef}
          className='text-muted-foreground hover:text-foreground hover:bg-muted inline-flex h-5 shrink-0 items-center gap-0.5 rounded-md border border-dashed px-1.5 text-xs'
        >
          <Plus className='size-3' aria-hidden='true' />
          {t('Add group')}
        </PopoverTrigger>
      )}
      <PopoverContent
        align='start'
        anchor={anchorRef}
        collisionPadding={8}
        className='w-[min(28rem,calc(100vw-1rem))] gap-2 p-2'
      >
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
          <div
            data-slot='group-options'
            className='grid max-h-72 grid-cols-[repeat(auto-fill,minmax(11rem,1fr))] gap-1 overflow-y-auto'
          >
            {visibleGroups.map((group) => {
              const checked = groups.includes(group)
              const label = sensitiveVisible ? group : SENSITIVE_MASK
              // Not wrapped in a <label>: Base UI would link it through
              // aria-labelledby and read the name twice. The capsule only
              // toggles on its own clicks, so one click is one change.
              return (
                <div
                  key={group}
                  className='hover:bg-muted flex min-w-0 items-start gap-2 rounded-md px-1.5 py-1'
                >
                  <Checkbox
                    checked={checked}
                    disabled={isSaving}
                    onCheckedChange={(nextChecked) =>
                      void toggleGroup(group, nextChecked)
                    }
                    aria-label={label}
                    className='mt-0.5'
                  />
                  <span
                    aria-hidden='true'
                    onClick={() => {
                      if (!isSaving) {
                        void toggleGroup(group, !checked)
                      }
                    }}
                    className='min-w-0 cursor-pointer'
                  >
                    <GroupCapsule
                      group={group}
                      className='break-all whitespace-normal'
                    />
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
