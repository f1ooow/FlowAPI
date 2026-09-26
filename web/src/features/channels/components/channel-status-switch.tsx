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
import { AlertTriangle } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Switch } from '@/components/ui/switch'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { formatTimestampToDate } from '@/lib/format'

import { CHANNEL_STATUS, CHANNEL_STATUS_CONFIG } from '../constants'
import {
  handleDisableTagChannels,
  handleEnableTagChannels,
  handleToggleChannelStatus,
  isMultiKeyChannel,
  type TagRow,
} from '../lib'
import type { Channel } from '../types'

/**
 * Switch for every channel of a tag. On only when all of them are enabled;
 * a partly enabled tag reads off, shows the enabled count, and switching it
 * on enables the rest.
 */
export function TagStatusSwitch(props: { row: TagRow }) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [pendingEnabled, setPendingEnabled] = useState<boolean | null>(null)
  const tag = props.row.tag || ''
  const total = props.row.children.length
  const enabledCount = props.row.children.filter(
    (child) => child.status === CHANNEL_STATUS.ENABLED
  ).length
  const allEnabled = total > 0 && enabledCount === total

  const handleCheckedChange = async (checked: boolean) => {
    setPendingEnabled(checked)
    try {
      if (checked) {
        await handleEnableTagChannels(tag, queryClient)
      } else {
        await handleDisableTagChannels(tag, queryClient)
      }
    } finally {
      setPendingEnabled(null)
    }
  }

  return (
    <div className='flex items-center gap-1'>
      <Tooltip>
        <TooltipTrigger render={<span className='inline-flex px-1' />}>
          <Switch
            size='sm'
            checked={pendingEnabled ?? allEnabled}
            disabled={!tag || pendingEnabled !== null}
            onCheckedChange={(checked) => void handleCheckedChange(checked)}
            aria-label={t('Enable All')}
          />
        </TooltipTrigger>
        <TooltipContent>
          {allEnabled ? t('Disable All') : t('Enable All')}
        </TooltipContent>
      </Tooltip>
      <span
        className='text-muted-foreground font-mono text-xs tabular-nums'
        title={t('Enabled channels: {{enabled}}/{{total}}', {
          enabled: enabledCount,
          total,
        })}
      >
        {enabledCount}/{total}
      </span>
    </div>
  )
}

/**
 * On/off switch for a channel, replacing the status column and the old
 * enable/disable button. On means enabled; both manual and automatic disabling
 * read as off, so an automatic (or unknown) off state keeps a warning marker
 * with the reason and time the health check recorded.
 */
export function ChannelStatusSwitch(props: { channel: Channel }) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  // Holds the requested position until the refetched row reports it, so the
  // switch does not bounce back while the request is in flight.
  const [pendingEnabled, setPendingEnabled] = useState<boolean | null>(null)
  const status = props.channel.status
  const statusConfig =
    CHANNEL_STATUS_CONFIG[status as keyof typeof CHANNEL_STATUS_CONFIG] ??
    CHANNEL_STATUS_CONFIG[CHANNEL_STATUS.UNKNOWN]
  const statusLabel = t(statusConfig.label)
  const needsAttention =
    status !== CHANNEL_STATUS.ENABLED &&
    status !== CHANNEL_STATUS.MANUAL_DISABLED

  let statusReason = ''
  let statusTime = ''
  if (status === CHANNEL_STATUS.AUTO_DISABLED && props.channel.other_info) {
    try {
      const otherInfo = JSON.parse(props.channel.other_info)
      statusReason = otherInfo?.status_reason || ''
      statusTime = otherInfo?.status_time
        ? formatTimestampToDate(otherInfo.status_time)
        : ''
    } catch {
      // Malformed other_info only loses the details, not the marker.
    }
  }

  const channelInfo = props.channel.channel_info
  const keyCount = isMultiKeyChannel(props.channel)
    ? (channelInfo?.multi_key_size ?? 0)
    : 0
  const enabledKeyCount = Math.max(
    0,
    keyCount - Object.keys(channelInfo?.multi_key_status_list ?? {}).length
  )

  const handleCheckedChange = async (checked: boolean) => {
    setPendingEnabled(checked)
    try {
      await handleToggleChannelStatus(props.channel.id, status, queryClient)
    } finally {
      setPendingEnabled(null)
    }
  }

  return (
    <div className='flex items-center gap-1'>
      <Tooltip>
        <TooltipTrigger render={<span className='inline-flex px-1' />}>
          <Switch
            size='sm'
            checked={pendingEnabled ?? status === CHANNEL_STATUS.ENABLED}
            disabled={pendingEnabled !== null}
            onCheckedChange={(checked) => void handleCheckedChange(checked)}
            aria-label={t('Enable')}
          />
        </TooltipTrigger>
        <TooltipContent>{statusLabel}</TooltipContent>
      </Tooltip>
      {needsAttention && (
        <Tooltip>
          <TooltipTrigger
            render={
              <span
                role='img'
                aria-label={statusLabel}
                className='inline-flex text-amber-500'
              />
            }
          >
            <AlertTriangle className='size-3.5' />
          </TooltipTrigger>
          <TooltipContent side='top' className='max-w-xs'>
            <div className='space-y-1 text-xs'>
              <div>{statusLabel}</div>
              {statusReason && (
                <div>
                  {t('Reason:')} {statusReason}
                </div>
              )}
              {statusTime && (
                <div>
                  {t('Time:')} {statusTime}
                </div>
              )}
            </div>
          </TooltipContent>
        </Tooltip>
      )}
      {keyCount > 0 && (
        <span
          className='text-muted-foreground font-mono text-xs tabular-nums'
          title={t('Available keys: {{enabled}}/{{total}}', {
            enabled: enabledKeyCount,
            total: keyCount,
          })}
        >
          {enabledKeyCount}/{keyCount}
        </span>
      )}
    </div>
  )
}
