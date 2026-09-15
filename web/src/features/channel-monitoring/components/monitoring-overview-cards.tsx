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

import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { cn } from '@/lib/utils'

import { formatMilliseconds } from '../lib/format'
import type { ChannelMonitoringOverall } from '../types'

const NO_VALUE = '--'

function OverviewCard(props: {
  title: string
  value: string
  valueClassName?: string
}) {
  return (
    <Card size='sm' className='rounded-lg'>
      <CardHeader>
        <CardTitle className='text-muted-foreground text-xs font-normal uppercase'>
          {props.title}
        </CardTitle>
      </CardHeader>
      <CardContent>
        <div
          className={cn(
            'font-mono text-2xl leading-none font-semibold',
            props.valueClassName
          )}
        >
          {props.value}
        </div>
      </CardContent>
    </Card>
  )
}

/**
 * Window totals across every channel. They are computed by the backend from
 * summed counters, so they stay correct even when a channel contributes a
 * single attempt.
 */
export function MonitoringOverviewCards(props: {
  overall: ChannelMonitoringOverall
}) {
  const { t } = useTranslation()
  const hasData = props.overall.has_data

  let availabilityClassName = 'text-muted-foreground'
  if (hasData) {
    availabilityClassName =
      props.overall.availability_rate >= 99
        ? 'text-emerald-600 dark:text-emerald-400'
        : 'text-amber-600 dark:text-amber-400'
  }

  // Cache has its own emptiness: the window can be full of attempts and still
  // contain no request that reported cache usage.
  const hasCacheData = props.overall.has_cache_data

  return (
    <div className='grid gap-3 sm:grid-cols-2 lg:grid-cols-4'>
      <OverviewCard
        title={t('Overall availability')}
        value={
          hasData ? `${props.overall.availability_rate.toFixed(2)}%` : NO_VALUE
        }
        valueClassName={availabilityClassName}
      />
      <OverviewCard
        title={t('Average latency')}
        value={
          hasData
            ? formatMilliseconds(props.overall.avg_latency_ms, t)
            : NO_VALUE
        }
      />
      <OverviewCard
        title={t('Error rate')}
        value={hasData ? `${props.overall.error_rate.toFixed(2)}%` : NO_VALUE}
      />
      <OverviewCard
        title={t('Cache hit rate')}
        value={
          hasCacheData
            ? `${props.overall.cache_hit_rate.toFixed(2)}%`
            : NO_VALUE
        }
      />
    </div>
  )
}
