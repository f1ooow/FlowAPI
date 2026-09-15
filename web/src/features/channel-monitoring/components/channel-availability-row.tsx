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
import { Card, CardContent, CardHeader } from '@/components/ui/card'
import { toIntlLocale } from '@/i18n/languages'
import { formatQuotaWithCurrency, getCurrencyLabel } from '@/lib/currency'
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
const MAX_DAILY_QUOTA_CHARS = 14

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
 * One channel card: identity and state first, then the window totals and the
 * availability timeline.
 *
 * The number of timeline slots is whatever the API returned in `buckets`; the
 * backend sizes that array from the range and the effective bucket width, so no
 * column count may be hard-coded here.
 */
export function ChannelAvailabilityRow(props: {
  channel: ChannelMonitoringChannelSummary
}) {
  const { t, i18n } = useTranslation()
  const hasData = props.channel.has_data
  // A separate flag on purpose: a channel can serve plenty of attempts and
  // still have no cache sample at all, and 0% would read as "caching broken"
  // rather than "this provider never reports cache".
  const hasCacheData = props.channel.has_cache_data
  // A channel deleted after producing samples keeps its metrics but loses its
  // name, so the id is the only identity left to show.
  const isDeleted = props.channel.channel_name === ''
  const displayName = isDeleted
    ? `#${props.channel.channel_id}`
    : props.channel.channel_name
  let todayUsedQuota = NO_VALUE
  let todayUsedQuotaTitle = NO_VALUE
  if (props.channel.today_used_quota != null) {
    const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
    const tokenSuffix = getCurrencyLabel() === 'Tokens' ? ' Tokens' : ''
    const fullValue = `${formatQuotaWithCurrency(
      props.channel.today_used_quota,
      {
        digitsLarge: 2,
        digitsSmall: 4,
        abbreviate: false,
        locale,
      }
    )}${tokenSuffix}`
    todayUsedQuota = fullValue
    todayUsedQuotaTitle = fullValue
    if (fullValue.length > MAX_DAILY_QUOTA_CHARS) {
      todayUsedQuota = `${formatQuotaWithCurrency(
        props.channel.today_used_quota,
        {
          compact: true,
          locale,
        }
      )}${tokenSuffix}`
    }
  }

  return (
    <Card size='sm' className='rounded-lg' role='listitem'>
      <CardHeader className='grid grid-cols-[minmax(0,1fr)_auto] items-start gap-3'>
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
        </div>
        <Badge
          variant='outline'
          data-state={props.channel.state}
          className={cn('shrink-0', stateTextClassName(props.channel.state))}
        >
          {t(stateLabelKey(props.channel.state))}
        </Badge>
      </CardHeader>

      <CardContent>
        <dl className='grid grid-cols-2 gap-x-4 gap-y-3'>
          <div className='min-w-0'>
            <dt className='text-muted-foreground text-[11px] leading-4'>
              {t('Availability')}
            </dt>
            <dd
              className={cn(
                'mt-1 whitespace-nowrap font-mono text-xl leading-none font-semibold tabular-nums',
                stateTextClassName(props.channel.state)
              )}
              title={
                hasData
                  ? `${props.channel.availability_rate.toFixed(2)}%`
                  : NO_VALUE
              }
            >
              {hasData
                ? `${props.channel.availability_rate.toFixed(2)}%`
                : NO_VALUE}
            </dd>
          </div>
          <div className='min-w-0'>
            <dt className='text-muted-foreground text-[11px] leading-4'>
              {t('Avg latency')}
            </dt>
            <dd className='mt-1 font-mono text-sm leading-none font-semibold whitespace-nowrap tabular-nums'>
              {hasData
                ? formatMilliseconds(props.channel.avg_latency_ms, t)
                : NO_VALUE}
            </dd>
          </div>
          <div className='min-w-0'>
            <dt className='text-muted-foreground text-[11px] leading-4'>
              {t('Attempts')}
            </dt>
            <dd
              className='mt-1 font-mono text-sm leading-none font-semibold whitespace-nowrap tabular-nums'
              title={props.channel.attempt_count.toLocaleString()}
            >
              {props.channel.attempt_count.toLocaleString()}
            </dd>
          </div>
          <div className='min-w-0'>
            <dt className='text-muted-foreground text-[11px] leading-4'>
              {t('Cache hit')}
            </dt>
            <dd className='mt-1 font-mono text-sm leading-none font-semibold whitespace-nowrap tabular-nums'>
              {hasCacheData
                ? `${props.channel.cache_hit_rate.toFixed(2)}%`
                : NO_VALUE}
            </dd>
          </div>
          <div className='col-span-2 flex min-w-0 items-baseline justify-between gap-3 border-t pt-3'>
            <dt className='text-muted-foreground shrink-0 text-[11px] leading-4'>
              {t('Consumed today')}
            </dt>
            <dd
              className='min-w-0 truncate font-mono text-base leading-none font-semibold tabular-nums'
              title={todayUsedQuotaTitle}
            >
              {todayUsedQuota}
            </dd>
          </div>
        </dl>

        <div
          className='mt-4 grid h-6 auto-cols-fr grid-flow-col gap-0 sm:gap-px'
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
      </CardContent>
    </Card>
  )
}
