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

import { Button } from '@/components/ui/button'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { ChannelAvailabilityRow } from '@/features/channel-monitoring/components/channel-availability-row'
import { RangeSelector } from '@/features/channel-monitoring/components/range-selector'

import { useChannelHealth } from './channel-health-context'

export function ChannelHealthDrawer() {
  const { t } = useTranslation()
  const health = useChannelHealth()
  const channel = health.selectedChannel
  const displayName = channel?.channel_name || `#${channel?.channel_id ?? ''}`

  return (
    <Sheet
      open={channel !== undefined}
      onOpenChange={(open) => {
        if (!open) health.closeChannel()
      }}
    >
      <SheetContent className='w-full overflow-y-auto sm:max-w-2xl'>
        <SheetHeader className='border-b'>
          <SheetTitle>{displayName}</SheetTitle>
          <SheetDescription>
            {t('Channel health over the selected time range')}
          </SheetDescription>
          <RangeSelector
            ranges={health.summary?.ranges ?? []}
            value={health.summary?.range ?? health.range}
            onChange={health.setRange}
          />
        </SheetHeader>
        <div className='space-y-4 overflow-y-auto p-4'>
          {health.isError && (
            <div className='text-muted-foreground flex items-center justify-between gap-3 rounded-md border p-3 text-sm'>
              <span>{t('Health data unavailable')}</span>
              <Button variant='outline' size='sm' onClick={health.retry}>
                {t('Retry')}
              </Button>
            </div>
          )}
          {channel && !health.isError && (
            <ChannelAvailabilityRow channel={channel} />
          )}
        </div>
      </SheetContent>
    </Sheet>
  )
}
