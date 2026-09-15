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
import { useQuery } from '@tanstack/react-query'
import { Radio, Search } from 'lucide-react'
import { useMemo, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { SectionPageLayout } from '@/components/layout'
import { Button } from '@/components/ui/button'
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from '@/components/ui/empty'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'

import { getChannelMonitoringSummary } from './api'
import { ChannelAvailabilityRow } from './components/channel-availability-row'
import { MonitoringOverviewCards } from './components/monitoring-overview-cards'
import { RangeSelector } from './components/range-selector'

// channel_metrics is flushed on the perf_metrics ticker, so polling faster than
// a minute only re-renders identical data.
const SUMMARY_REFETCH_INTERVAL_MS = 60_000
const DEFAULT_RANGE = '24h'

export function ChannelMonitoring() {
  const { t } = useTranslation()
  const [range, setRange] = useState(DEFAULT_RANGE)
  const [search, setSearch] = useState('')

  const summaryQuery = useQuery({
    queryKey: ['channel-monitoring', 'summary', range],
    queryFn: () => getChannelMonitoringSummary(range),
    refetchInterval: SUMMARY_REFETCH_INTERVAL_MS,
    // Keep the previous window on screen while a new range loads, so the range
    // switcher does not disappear between requests.
    placeholderData: (previous) => previous,
  })

  const channels = useMemo(
    () => summaryQuery.data?.channels ?? [],
    [summaryQuery.data]
  )

  const trafficChannels = useMemo(
    () => channels.filter((channel) => channel.has_data),
    [channels]
  )

  const visibleChannels = useMemo(() => {
    const keyword = search.trim().toLocaleLowerCase()
    return trafficChannels.filter((channel) => {
      if (!keyword) return true
      return (
        channel.channel_name.toLocaleLowerCase().includes(keyword) ||
        String(channel.channel_id).includes(keyword)
      )
    })
  }, [search, trafficChannels])

  const issueCount = trafficChannels.filter(
    (channel) => channel.state === 'down' || channel.state === 'degraded'
  ).length
  const activeCount = trafficChannels.length

  let monitoringContent: ReactNode
  if (summaryQuery.isLoading) {
    monitoringContent = (
      <div className='grid gap-3 md:grid-cols-2 xl:grid-cols-3'>
        {[0, 1, 2, 3, 4, 5].map((item) => (
          <Skeleton key={item} className='h-52 rounded-lg' />
        ))}
      </div>
    )
  } else if (summaryQuery.isError) {
    monitoringContent = (
      <Empty className='min-h-64 border'>
        <EmptyHeader>
          <EmptyMedia variant='icon'>
            <Radio aria-hidden='true' />
          </EmptyMedia>
          <EmptyTitle>{t('Failed to load monitoring data')}</EmptyTitle>
          <EmptyDescription>
            <Button
              variant='outline'
              size='sm'
              onClick={() => summaryQuery.refetch()}
            >
              {t('Retry')}
            </Button>
          </EmptyDescription>
        </EmptyHeader>
      </Empty>
    )
  } else if (visibleChannels.length > 0) {
    monitoringContent = (
      <div
        className='grid gap-3 md:grid-cols-2 xl:grid-cols-3'
        role='list'
        aria-label={t('Channel Availability')}
      >
        {visibleChannels.map((channel) => (
          <ChannelAvailabilityRow key={channel.channel_id} channel={channel} />
        ))}
      </div>
    )
  } else {
    let emptyDescription = t('No channel has recorded an attempt yet')
    if (search) {
      emptyDescription = t('No channels match your search')
    } else if (channels.length > 0) {
      emptyDescription = t('No channel received traffic in this window')
    }
    monitoringContent = (
      <Empty className='min-h-64 border'>
        <EmptyHeader>
          <EmptyMedia variant='icon'>
            <Radio aria-hidden='true' />
          </EmptyMedia>
          <EmptyTitle>{t('No channels to show')}</EmptyTitle>
          <EmptyDescription>{emptyDescription}</EmptyDescription>
        </EmptyHeader>
      </Empty>
    )
  }

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>
        <span className='inline-flex min-w-0 items-center gap-3'>
          <span className='truncate'>{t('Channel Availability')}</span>
          <span className='text-muted-foreground hidden items-center gap-3 text-xs font-normal sm:inline-flex'>
            <span>{t('{{count}} with traffic', { count: activeCount })}</span>
            <span>{t('{{count}} with issues', { count: issueCount })}</span>
          </span>
        </span>
      </SectionPageLayout.Title>
      <SectionPageLayout.Actions>
        <RangeSelector
          ranges={summaryQuery.data?.ranges ?? []}
          value={summaryQuery.data?.range ?? range}
          onChange={setRange}
        />
      </SectionPageLayout.Actions>
      <SectionPageLayout.Content>
        <div className='space-y-4'>
          <MonitoringOverviewCards
            overall={
              summaryQuery.data?.overall ?? {
                has_data: false,
                availability_rate: 0,
                error_rate: 0,
                avg_latency_ms: 0,
                attempt_count: 0,
                has_cache_data: false,
                cache_hit_rate: 0,
                cache_engagement_rate: 0,
                cache_request_count: 0,
                cache_signal_count: 0,
                cache_read_tokens: 0,
                cache_write_tokens: 0,
                cache_input_tokens: 0,
              }
            }
          />

          <div className='flex justify-end'>
            <div className='relative w-full sm:w-72'>
              <Search
                className='text-muted-foreground pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2'
                aria-hidden='true'
              />
              <Input
                className='pl-8'
                value={search}
                onChange={(event) => setSearch(event.target.value)}
                placeholder={t('Search channels')}
                aria-label={t('Search channels')}
              />
            </div>
          </div>

          {monitoringContent}
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
