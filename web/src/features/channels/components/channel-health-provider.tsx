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
import { useCallback, useMemo, useState, type ReactNode } from 'react'

import { getChannelMonitoringSummary } from '@/features/channel-monitoring/api'

import {
  ChannelHealthContext,
  type ChannelHealthContextValue,
} from './channel-health-context'

const DEFAULT_RANGE = '24h'
const SUMMARY_REFETCH_INTERVAL_MS = 60_000

export function ChannelHealthProvider(props: { children: ReactNode }) {
  const [range, setRange] = useState(DEFAULT_RANGE)
  const [selectedChannelId, setSelectedChannelId] = useState<number | null>(
    null
  )
  const summaryQuery = useQuery({
    queryKey: ['channel-monitoring', 'summary', range],
    queryFn: () => getChannelMonitoringSummary(range),
    refetchInterval: SUMMARY_REFETCH_INTERVAL_MS,
    staleTime: SUMMARY_REFETCH_INTERVAL_MS,
    placeholderData: (previous) => previous,
  })
  const { refetch } = summaryQuery
  const healthByChannelId = useMemo(
    () =>
      new Map(
        (summaryQuery.data?.channels ?? []).map((channel) => [
          channel.channel_id,
          channel,
        ])
      ),
    [summaryQuery.data]
  )
  const openChannel = useCallback((channelId: number) => {
    setSelectedChannelId(channelId)
  }, [])
  const closeChannel = useCallback(() => {
    setSelectedChannelId(null)
  }, [])
  const retry = useCallback(() => {
    void refetch()
  }, [refetch])
  const value = useMemo<ChannelHealthContextValue>(
    () => ({
      summary: summaryQuery.data,
      healthByChannelId,
      selectedChannel:
        selectedChannelId === null
          ? undefined
          : healthByChannelId.get(selectedChannelId),
      range,
      setRange,
      openChannel,
      closeChannel,
      isLoading: summaryQuery.isLoading,
      isUpdating: summaryQuery.isFetching,
      isError: summaryQuery.isError,
      retry,
    }),
    [
      closeChannel,
      healthByChannelId,
      openChannel,
      range,
      retry,
      selectedChannelId,
      summaryQuery.data,
      summaryQuery.isError,
      summaryQuery.isLoading,
      summaryQuery.isFetching,
    ]
  )

  return (
    <ChannelHealthContext.Provider value={value}>
      {props.children}
    </ChannelHealthContext.Provider>
  )
}
