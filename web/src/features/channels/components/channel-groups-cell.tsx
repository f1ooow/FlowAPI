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
import { useState } from 'react'
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

import { useChannelGroups } from '../hooks/use-channel-groups'
import { handleUpdateChannelGroups, parseGroupsList } from '../lib'
import type { Channel } from '../types'
import { useChannels } from './channels-provider'

const SENSITIVE_MASK = '••••'

/**
 * Groups named in the cell before the rest collapse into a "+N" count. A fixed
 * number rather than "as many as fit", so the summary reads the same on every
 * row and never depends on measuring the text.
 */
const NAMED_GROUP_COUNT = 2

/**
 * The groups of a channel as one quiet line of text: the first
 * {@link NAMED_GROUP_COUNT} names, then "+N" for the rest. The full list is in
 * the title, and a click opens a searchable checkbox grid of every group where
 * each tick saves the channel's groups right away. A channel without groups
 * shows an outlined "Add group" button that opens the same grid.
 *
 * The line's intrinsic width is a constant, whatever the groups: `w-24` is all
 * it asks of a table column, and `min-w-full` lets it fill whatever width the
 * column has. So the column is exactly as wide with one group as with a
 * hundred, a tight table cannot squeeze it below that constant, and the names
 * truncate inside the width instead of widening the column or wrapping the row.
 */
export function ChannelGroupsCell(props: { channel: Channel }) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const { sensitiveVisible } = useChannels()
  const [open, setOpen] = useState(false)
  const [search, setSearch] = useState('')
  const [isSaving, setIsSaving] = useState(false)
  const { data: groupsData } = useChannelGroups()
  const groups = parseGroupsList(props.channel.group)
  const labels = groups.map((group) =>
    sensitiveVisible ? group : SENSITIVE_MASK
  )
  const namedLabels = labels.slice(0, NAMED_GROUP_COUNT)
  const collapsedCount = labels.length - namedLabels.length
  const fullList = labels.join(', ')

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
      {/* One trigger element for both states, so ticking the first group into
          an empty channel keeps the open panel anchored where it opened. */}
      <PopoverTrigger
        data-slot='channel-groups'
        aria-label={
          groups.length > 0
            ? t('Edit groups: {{group}}', { group: fullList })
            : undefined
        }
        title={groups.length > 0 ? fullList : undefined}
        className={cn(
          'text-muted-foreground hover:text-foreground focus-visible:ring-ring/50 cursor-pointer items-center rounded-md text-xs whitespace-nowrap outline-none focus-visible:ring-2',
          groups.length > 0
            ? 'flex h-6 w-24 min-w-full gap-1.5 text-left'
            : 'hover:bg-muted inline-flex h-5 gap-0.5 border border-dashed px-1.5'
        )}
      >
        {groups.length > 0 ? (
          <>
            <span className='min-w-0 truncate'>{namedLabels.join(', ')}</span>
            {collapsedCount > 0 && (
              <span className='bg-muted shrink-0 rounded px-1 leading-4 tabular-nums'>
                +{collapsedCount}
              </span>
            )}
          </>
        ) : (
          <>
            <Plus className='size-3' aria-hidden='true' />
            {t('Add group')}
          </>
        )}
      </PopoverTrigger>
      <PopoverContent
        align='start'
        collisionPadding={8}
        // Behind this panel sits a dense table whose own rows look like the
        // list being edited, so the panel has to declare itself as a surface:
        // an opaque body, a crisp border and a real shadow. The shared popover
        // tone is a `ring-foreground/10` hairline with a soft shadow, which over
        // a white table reads as almost no edge at all, so the ring is dropped
        // in favour of an explicit border here.
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
                      weight and contrast, the same quiet language as the
                      cell that opened this panel. */}
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
