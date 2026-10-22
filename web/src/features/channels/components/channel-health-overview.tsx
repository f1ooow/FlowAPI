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
import { Activity, ChartNoAxesCombined } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { RangeSelector } from '@/features/channel-monitoring/components/range-selector'
import { formatMilliseconds } from '@/features/channel-monitoring/lib/format'
import { formatTimestampToDate } from '@/lib/format'

import { useChannelHealth } from './channel-health-context'
import { ChannelHealthHistory } from './channel-health-history'

export function ChannelHealthOverview() {
  const { t } = useTranslation()
  const health = useChannelHealth()
  const [historyOpen, setHistoryOpen] = useState(false)
  const overall = health.summary?.overall
  const hasData = overall?.has_data === true
  const issueCount = health.summary?.channels.filter(
    (channel) =>
      channel.has_data &&
      (channel.state === 'down' || channel.state === 'degraded')
  ).length
  const metrics = [
    {
      label: t('Overall availability'),
      value: hasData ? `${overall.availability_rate.toFixed(2)}%` : '--',
    },
    {
      label: t('Attempts'),
      value: overall?.attempt_count.toLocaleString() ?? '--',
    },
    {
      label: t('Average attempt latency'),
      value: hasData ? formatMilliseconds(overall.avg_latency_ms, t) : '--',
    },
    {
      label: t('Channels with issues'),
      value: issueCount?.toLocaleString() ?? '--',
    },
  ]

  return (
    <>
      <section
        aria-label={t('Channel health')}
        className='bg-muted/20 shrink-0 rounded-lg border'
      >
        <div className='flex flex-wrap items-center justify-between gap-x-4 gap-y-2 border-b px-3 py-2'>
          <div className='flex flex-wrap items-center gap-x-3 gap-y-1'>
            <span className='flex items-center gap-1.5 text-xs font-medium'>
              <Activity aria-hidden='true' className='size-3.5' />
              {t('Channel health')}
            </span>
            <span className='text-muted-foreground text-xs'>
              {t('Attempt-level health across all channels')}
            </span>
            {health.summary?.updated_at && (
              <span className='text-muted-foreground text-[11px]'>
                {t('Data through {{time}}', {
                  time: formatTimestampToDate(health.summary.updated_at),
                })}
              </span>
            )}
            {health.isUpdating && (
              <span className='text-muted-foreground text-xs'>
                {t('Updating...')}
              </span>
            )}
          </div>
          <div className='flex flex-wrap items-center gap-2'>
            <RangeSelector
              ranges={health.summary?.ranges ?? []}
              value={health.range}
              onChange={health.setRange}
            />
            <Button
              variant='ghost'
              size='sm'
              onClick={() => setHistoryOpen(true)}
            >
              <ChartNoAxesCombined aria-hidden='true' />
              {t('History and totals')}
            </Button>
          </div>
        </div>
        {health.isError ? (
          <div
            className='flex items-center justify-between gap-3 px-3 py-2 text-sm'
            role='status'
          >
            <span className='text-muted-foreground'>
              {t('Health data unavailable')}
            </span>
            <Button variant='outline' size='sm' onClick={health.retry}>
              {t('Retry')}
            </Button>
          </div>
        ) : (
          <dl className='grid grid-cols-2 gap-y-3 px-3 py-3 sm:grid-cols-4'>
            {metrics.map((metric) => (
              <div key={metric.label} className='min-w-0 px-2 first:pl-0'>
                <dt
                  className='text-muted-foreground truncate text-[11px]'
                  title={metric.label}
                >
                  {metric.label}
                </dt>
                <dd className='mt-1 font-mono text-base font-semibold tabular-nums'>
                  {health.isLoading ? (
                    <Skeleton className='h-5 w-16' />
                  ) : (
                    metric.value
                  )}
                </dd>
              </div>
            ))}
          </dl>
        )}
      </section>
      <ChannelHealthHistory open={historyOpen} onOpenChange={setHistoryOpen} />
    </>
  )
}
