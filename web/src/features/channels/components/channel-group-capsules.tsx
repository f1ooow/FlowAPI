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
 * a group has the same colour in every row and card. The label stays on one
 * line and truncates when the row is tight, so a long group name can never
 * wrap a table row onto a second line. The edit popover deliberately does not
 * use this pill: it lists group names as plain text.
 */
export function GroupCapsule(props: { group: string; className?: string }) {
  const { sensitiveVisible } = useChannels()
  const capsuleClassName = useGroupCapsuleClassName()
  const label = sensitiveVisible ? props.group : SENSITIVE_MASK
  return (
    <span
      data-slot='group-capsule'
      title={label}
      className={cn(
        // The pill hugs its label and clips it when the room runs out, so a
        // long group name can neither widen a table cell nor stretch a popover
        // column. `w-fit` with `max-w-full` is what keeps those two apart.
        'block max-w-full min-w-0 w-fit truncate rounded-md px-2 py-0.5 text-xs leading-4 font-medium whitespace-nowrap text-white',
        capsuleClassName(props.group),
        props.className
      )}
    >
      {label}
    </span>
  )
}

/**
 * The groups of a channel as capsules that sit inline after its name. Any
 * capsule opens a searchable checkbox grid of every group; each tick saves the
 * channel's groups right away. A channel without groups shows an outlined
 * "Add group" button that opens the same grid.
 *
 * The capsules join the parent's one-line name row, so they truncate instead
 * of wrapping it.
 */
export function ChannelGroupCapsules(props: {
  channel: Channel
  className?: string
}) {
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
            className={cn(
              'focus-visible:ring-ring/50 inline-flex min-w-0 cursor-pointer rounded-md outline-none focus-visible:ring-2',
              props.className
            )}
          >
            <GroupCapsule group={group} />
          </PopoverTrigger>
        ))
      ) : (
        <PopoverTrigger
          ref={anchorRef}
          className={cn(
            'text-muted-foreground hover:text-foreground hover:bg-muted inline-flex h-5 shrink-0 items-center gap-0.5 rounded-md border border-dashed px-1.5 text-xs whitespace-nowrap',
            props.className
          )}
        >
          <Plus className='size-3' aria-hidden='true' />
          {t('Add group')}
        </PopoverTrigger>
      )}
      <PopoverContent
        align='start'
        anchor={anchorRef}
        collisionPadding={8}
        // Behind this panel sits a dense table whose own names and capsules
        // look like the ones being edited, so the panel has to declare itself
        // as a surface: an opaque body, a crisp border and a real shadow. The
        // shared popover tone is a `ring-foreground/10` hairline with a soft
        // shadow, which over a white table reads as almost no edge at all, so
        // the ring is dropped in favour of an explicit border here.
        className='bg-popover border-border w-[min(26rem,calc(100vw-1rem))] gap-0 overflow-hidden border p-0 shadow-lg ring-0'
      >
        {/* The search box keeps its own inset, so the panel's edge and the
            field's edge do not sit on top of each other. */}
        <div className='p-2 pb-1.5'>
          <Input
            value={search}
            onChange={(event) => setSearch(event.target.value)}
            placeholder={t('Search groups...')}
            aria-label={t('Search groups...')}
            className='h-8 w-full'
          />
        </div>
        {/* A hairline seam under the search box, so the list reads as its own
            surface instead of bleeding into the field above it. */}
        <div className='bg-border/70 h-px shrink-0' />
        {visibleGroups.length === 0 ? (
          <p className='text-muted-foreground px-2 py-3 text-center text-xs'>
            {t('No results found.')}
          </p>
        ) : (
          <div
            data-slot='group-options'
            // Two equal columns: `minmax(0, 1fr)` lets a long group name clip
            // inside its own cell instead of widening the column.
            className='grid max-h-72 grid-cols-[repeat(2,minmax(0,1fr))] gap-x-1 gap-y-0.5 overflow-y-auto p-2 pt-1.5'
          >
            {visibleGroups.map((group) => {
              const checked = groups.includes(group)
              const label = sensitiveVisible ? group : SENSITIVE_MASK
              // Not wrapped in a <label>: Base UI would link it through
              // aria-labelledby and read the name twice. The row only toggles
              // on the label's own clicks, so one click is one change.
              return (
                <div
                  key={group}
                  // One fixed row height for every option, so the two columns
                  // stay on the same rhythm whatever the names look like.
                  className='hover:bg-muted flex h-8 min-w-0 items-center gap-2 rounded-md px-2 transition-colors duration-100'
                >
                  <Checkbox
                    checked={checked}
                    disabled={isSaving}
                    onCheckedChange={(nextChecked) =>
                      void toggleGroup(group, nextChecked)
                    }
                    aria-label={label}
                    className='shrink-0'
                  />
                  {/* Plain text, no colour: a selected group reads through
                      weight and contrast, so the list stays quiet and the
                      capsules outside this panel keep the colour language to
                      themselves. */}
                  <span
                    aria-hidden='true'
                    title={label}
                    onClick={() => {
                      if (!isSaving) {
                        void toggleGroup(group, !checked)
                      }
                    }}
                    className={cn(
                      'min-w-0 flex-1 cursor-pointer truncate text-xs whitespace-nowrap transition-colors duration-100',
                      checked
                        ? 'text-foreground font-medium'
                        : 'text-muted-foreground'
                    )}
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
