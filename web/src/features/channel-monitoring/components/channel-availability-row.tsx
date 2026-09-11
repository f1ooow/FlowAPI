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

import { Badge } from '@/components/ui/badge'
import { cn } from '@/lib/utils'

import { formatBucketTime, formatMilliseconds } from '../lib/format'
import {
  bucketClassName,
  stateDotClassName,
  stateLabelKey,
  stateTextClassName,
} from '../lib/status'
import type {
  ChannelMonitoringBucket,
  ChannelMonitoringChannelSummary,
} from '../types'

const NO_VALUE = '--'

function bucketTitle(
  bucket: ChannelMonitoringBucket,
  translate: (key: string, options?: Record<string, unknown>) => string
) {
  const time = formatBucketTime(bucket.ts)
  if (bucket.state === 'no-data') {
    return `${time} · ${translate('No data')}`
  }
  const rate = Math.round((bucket.success_count / bucket.attempt_count) * 100)
  return `${time} · ${rate}% (${bucket.success_count}/${bucket.attempt_count})`
}

/**
 * One channel row: identity on the left, the availability timeline underneath,
 * and the window totals at the end of the row.
 *
 * The number of timeline slots is whatever the API returned in `buckets`; the
 * backend sizes that array from the range and the effective bucket width, so no
 * column count may be hard-coded here.
 */
export function ChannelAvailabilityRow(props: {
  channel: ChannelMonitoringChannelSummary
  stepMinutes: number
}) {
  const { t } = useTranslation()
  const hasData = props.channel.has_data
  // A channel deleted after producing samples keeps its metrics but loses its
  // name, so the id is the only identity left to show.
  const isDeleted = props.channel.channel_name === ''
  const displayName = isDeleted
    ? `#${props.channel.channel_id}`
    : props.channel.channel_name

  return (
    <div className='rounded-lg border p-3'>
      <div className='flex flex-wrap items-center justify-between gap-x-4 gap-y-2'>
        <div className='flex min-w-0 items-center gap-2'>
          <span
            className={cn(
              'size-2 shrink-0 rounded-full',
              stateDotClassName(props.channel.state)
            )}
            aria-hidden='true'
          />
          <span className='truncate text-sm font-medium' title={displayName}>
            {displayName}
          </span>
          {!isDeleted && (
            <span className='text-muted-foreground shrink-0 font-mono text-xs'>
              #{props.channel.channel_id}
            </span>
          )}
          {isDeleted && (
            <Badge variant='outline' className='text-muted-foreground shrink-0'>
              {t('Deleted channel')}
            </Badge>
          )}
          <Badge
            variant='outline'
            data-state={props.channel.state}
            className={cn('shrink-0', stateTextClassName(props.channel.state))}
          >
            {t(stateLabelKey(props.channel.state))}
          </Badge>
        </div>

        <div className='flex shrink-0 items-baseline gap-x-6'>
          <div className='text-right'>
            <div
              className={cn(
                'font-mono text-sm leading-none font-semibold',
                stateTextClassName(props.channel.state)
              )}
            >
              {hasData
                ? `${props.channel.availability_rate.toFixed(2)}%`
                : NO_VALUE}
            </div>
            <div className='text-muted-foreground mt-1 text-[11px] uppercase'>
              {t('Availability')}
            </div>
          </div>
          <div className='text-right'>
            <div className='font-mono text-sm leading-none font-semibold'>
              {hasData
                ? formatMilliseconds(props.channel.avg_latency_ms, t)
                : NO_VALUE}
            </div>
            <div className='text-muted-foreground mt-1 text-[11px] uppercase'>
              {t('Avg latency')}
            </div>
          </div>
          <div className='text-right'>
            <div className='font-mono text-sm leading-none font-semibold'>
              {props.channel.attempt_count.toLocaleString()}
            </div>
            <div
              className='text-muted-foreground mt-1 text-[11px] uppercase'
              title={t(
                'Channel attempts, not user requests. One request that fails over is counted on every channel it tried.'
              )}
            >
              {t('Attempts')}
            </div>
          </div>
        </div>
      </div>

      <div
        className='mt-3 grid h-6 auto-cols-fr grid-flow-col gap-0 sm:gap-px'
        role='img'
        aria-label={t('Availability timeline for {{name}}', {
          name: displayName,
        })}
      >
        {props.channel.buckets.map((bucket) => (
          <span
            key={bucket.ts}
            data-state={bucket.state}
            className={cn(
              'min-w-0 rounded-[1px]',
              bucketClassName(bucket.state)
            )}
            title={bucketTitle(bucket, t)}
          />
        ))}
      </div>

      <div className='text-muted-foreground mt-2 flex items-center justify-between gap-2 text-[11px]'>
        <span>
          {hasData
            ? t('{{success}} of {{total}} attempts succeeded', {
                success: props.channel.success_count.toLocaleString(),
                total: props.channel.attempt_count.toLocaleString(),
              })
            : t('No attempts in this window')}
        </span>
        <span>
          {t('{{minutes}} min per bar', { minutes: props.stepMinutes })}
        </span>
      </div>
    </div>
  )
}
