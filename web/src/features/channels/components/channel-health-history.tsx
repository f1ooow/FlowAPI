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
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { Skeleton } from '@/components/ui/skeleton'
import { getChannelMonitoringTotals } from '@/features/channel-monitoring/api'
import { ChannelAvailabilityRow } from '@/features/channel-monitoring/components/channel-availability-row'
import {
  ChannelMonitoringTotalsBoard,
  type TotalsWindowKey,
} from '@/features/channel-monitoring/components/channel-monitoring-totals-board'
import { RangeSelector } from '@/features/channel-monitoring/components/range-selector'

import { useChannelHealth } from './channel-health-context'

export function ChannelHealthHistory(props: {
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const { t } = useTranslation()
  const health = useChannelHealth()
  const [search, setSearch] = useState('')
  const [totalsWindow, setTotalsWindow] = useState<TotalsWindowKey>('today')
  const totalsQuery = useQuery({
    queryKey: ['channel-monitoring', 'totals'],
    queryFn: getChannelMonitoringTotals,
    enabled: props.open,
    refetchInterval: props.open ? 60_000 : false,
    staleTime: 60_000,
  })
  const totals = totalsQuery.data?.available
    ? totalsQuery.data.totals
    : undefined
  const keyword = search.trim().toLocaleLowerCase()
  const visibleChannels = (health.summary?.channels ?? []).filter(
    (channel) =>
      channel.has_data &&
      (!keyword ||
        channel.channel_name.toLocaleLowerCase().includes(keyword) ||
        String(channel.channel_id).includes(keyword))
  )

  return (
    <Sheet open={props.open} onOpenChange={props.onOpenChange}>
      <SheetContent className='w-full overflow-y-auto sm:max-w-5xl'>
        <SheetHeader className='border-b'>
          <SheetTitle>{t('History and totals')}</SheetTitle>
          <SheetDescription>
            {t(
              'Fleet totals include deleted channels. Health history follows the selected time range.'
            )}
          </SheetDescription>
        </SheetHeader>
        <div className='space-y-4 overflow-y-auto p-4'>
          {totalsQuery.isLoading && <Skeleton className='h-40 w-full' />}
          {totalsQuery.isError && (
            <div className='text-muted-foreground flex items-center justify-between gap-3 rounded-md border p-3 text-sm'>
              {t('Failed to load fleet totals')}
              <Button
                variant='outline'
                size='sm'
                onClick={() => void totalsQuery.refetch()}
              >
                {t('Retry')}
              </Button>
            </div>
          )}
          {!totalsQuery.isLoading && !totalsQuery.isError && (
            <ChannelMonitoringTotalsBoard
              windows={{
                today: totals?.today,
                month: totals?.month,
                total: totals?.total,
              }}
              selectedWindow={totalsWindow}
              onSelectWindow={setTotalsWindow}
            />
          )}
          <div className='flex flex-wrap items-center justify-between gap-3'>
            <RangeSelector
              ranges={health.summary?.ranges ?? []}
              value={health.range}
              onChange={health.setRange}
            />
            <Input
              className='w-full sm:w-64'
              value={search}
              onChange={(event) => setSearch(event.target.value)}
              placeholder={t('Search channels')}
              aria-label={t('Search channels')}
            />
          </div>
          {health.isError ? (
            <div className='text-muted-foreground flex items-center justify-between gap-3 rounded-md border p-3 text-sm'>
              {t('Health data unavailable')}
              <Button variant='outline' size='sm' onClick={health.retry}>
                {t('Retry')}
              </Button>
            </div>
          ) : (
            <div
              role='list'
              aria-label={t('Channel Availability')}
              className='grid gap-3 md:grid-cols-2'
            >
              {visibleChannels.map((channel) => (
                <ChannelAvailabilityRow
                  key={channel.channel_id}
                  channel={channel}
                />
              ))}
              {visibleChannels.length === 0 && (
                <p className='text-muted-foreground py-8 text-center text-sm md:col-span-2'>
                  {t('No channel received traffic in this window')}
                </p>
              )}
            </div>
          )}
        </div>
      </SheetContent>
    </Sheet>
  )
}
