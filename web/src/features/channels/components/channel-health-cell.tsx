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
import { ChevronRight } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Skeleton } from '@/components/ui/skeleton'
import {
  formatBucketTime,
  formatMilliseconds,
} from '@/features/channel-monitoring/lib/format'
import {
  bucketClassName,
  stateDotClassName,
  stateLabelKey,
  stateTextClassName,
} from '@/features/channel-monitoring/lib/status'
import { cn } from '@/lib/utils'

import { isTagAggregateRow, type TagRow } from '../lib'
import type { Channel } from '../types'
import { useChannelHealth } from './channel-health-context'

export function ChannelHealthCell(props: {
  channel: Channel | TagRow
  onExpand?: () => void
}) {
  const { t } = useTranslation()
  const health = useChannelHealth()

  if (isTagAggregateRow(props.channel)) {
    return (
      <button
        type='button'
        onClick={props.onExpand}
        className='text-muted-foreground hover:text-foreground flex min-h-11 items-center gap-1 text-left text-xs'
      >
        {t('Expand to view channel health')}
        <ChevronRight aria-hidden='true' className='size-3.5 shrink-0' />
      </button>
    )
  }

  if (health.isLoading) {
    return (
      <div aria-label={t('Loading channel health')} className='space-y-2 py-1'>
        <Skeleton className='h-4 w-36' />
        <Skeleton className='h-2 w-full' />
      </div>
    )
  }

  if (health.isError) {
    return (
      <span className='text-muted-foreground inline-block py-3 text-xs'>
        {t('Health data unavailable')}
      </span>
    )
  }

  const summary = health.healthByChannelId.get(props.channel.id)
  const hasData = summary?.has_data === true
  const state = summary?.state ?? 'no-data'
  const availability = hasData
    ? `${summary.availability_rate.toFixed(2)}%`
    : '--'

  return (
    <button
      type='button'
      aria-label={t('View channel health for {{name}}', {
        name: props.channel.name,
      })}
      disabled={!summary}
      onClick={() => health.openChannel(props.channel.id)}
      className='hover:bg-muted/70 focus-visible:ring-ring/50 flex min-h-11 w-full min-w-0 flex-col gap-1.5 rounded-md px-1.5 py-1.5 text-left transition-colors outline-none focus-visible:ring-2 disabled:cursor-default disabled:hover:bg-transparent'
    >
      <span className='flex w-full items-center justify-between gap-3 text-xs'>
        <span className='flex min-w-0 items-center gap-1.5'>
          <span
            aria-hidden='true'
            className={cn(
              'size-1.5 shrink-0 rounded-full',
              stateDotClassName(state)
            )}
          />
          <span className={cn('truncate', stateTextClassName(state))}>
            {t(stateLabelKey(state))}
          </span>
        </span>
        <span
          className={cn(
            'shrink-0 font-mono font-semibold tabular-nums',
            stateTextClassName(state)
          )}
          title={t('Availability')}
        >
          {availability}
        </span>
      </span>
      <span
        role='img'
        aria-label={t('Availability timeline for {{name}}', {
          name: props.channel.name,
        })}
        className='bg-muted grid h-2 w-full auto-cols-fr grid-flow-col gap-px overflow-hidden rounded-xs'
      >
        {summary?.buckets.map((bucket) => (
          <span
            key={bucket.ts}
            data-state={bucket.state}
            className={cn('min-w-0', bucketClassName(bucket.state))}
            title={`${formatBucketTime(bucket.ts)} · ${t(stateLabelKey(bucket.state))} · ${bucket.success_count}/${bucket.attempt_count}`}
          />
        ))}
      </span>
      <span className='text-muted-foreground flex w-full items-center justify-between gap-2 text-[11px]'>
        <span title={t('Attempts')}>
          <span className='font-mono tabular-nums'>
            {summary?.attempt_count.toLocaleString() ?? '--'}
          </span>{' '}
          {t('Attempts')}
        </span>
        <span
          title={t('Average attempt latency')}
          className='shrink-0 font-mono tabular-nums'
        >
          {hasData ? formatMilliseconds(summary.avg_latency_ms, t) : '--'}
        </span>
      </span>
    </button>
  )
}
