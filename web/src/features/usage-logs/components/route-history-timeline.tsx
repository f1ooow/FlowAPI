/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the License,
or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.
*/
import { CheckCircle2, CircleStop, RefreshCw, XCircle } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { cn } from '@/lib/utils'

import type { RouteHistoryAttempt } from '../types'

interface RouteHistoryTimelineProps {
  attempts: RouteHistoryAttempt[]
  fallbackChannelName?: string
  showChannelNames?: boolean
  compact?: boolean
}

function AttemptTiming(props: { attempt: RouteHistoryAttempt }) {
  const { t, i18n } = useTranslation()
  const timing = props.attempt.timing
  if (
    !timing ||
    !Object.values(timing).every(
      (value) => Number.isSafeInteger(value) && value >= 0
    ) ||
    !Number.isSafeInteger(timing.duration_ms) ||
    !Number.isSafeInteger(timing.start_offset_ms) ||
    !Number.isSafeInteger(timing.end_offset_ms) ||
    !Number.isFinite(new Date(timing.started_at_ms).getTime()) ||
    !Number.isFinite(new Date(timing.ended_at_ms).getTime()) ||
    timing.end_offset_ms < timing.start_offset_ms ||
    timing.ended_at_ms < timing.started_at_ms
  ) {
    return (
      <div className='text-muted-foreground mt-0.5 text-[10px]'>
        {t('Timing not recorded')}
      </div>
    )
  }
  let event = t('Failed at +{{seconds}} s', {
    seconds: (timing.end_offset_ms / 1000).toFixed(2),
  })
  if (props.attempt.outcome === 'succeeded') {
    event = t('Success recorded at +{{seconds}} s', {
      seconds: (timing.end_offset_ms / 1000).toFixed(2),
    })
  } else if (props.attempt.outcome === 'stopped') {
    event = t('Stopped at +{{seconds}} s', {
      seconds: (timing.end_offset_ms / 1000).toFixed(2),
    })
  }
  const formatter = new Intl.DateTimeFormat(
    i18n.language === 'zhCN' ? 'zh-CN' : i18n.language,
    {
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit',
      fractionalSecondDigits: 3,
      hourCycle: 'h23',
      timeZoneName: 'longOffset',
    }
  )
  return (
    <Tooltip>
      <TooltipTrigger
        render={<button type='button' />}
        className='text-muted-foreground mt-0.5 block max-w-full text-left text-[10px] break-words underline decoration-dotted underline-offset-2'
      >
        {event}
        {' · '}
        {t('Attempt duration {{seconds}} s', {
          seconds: (timing.duration_ms / 1000).toFixed(2),
        })}
      </TooltipTrigger>
      <TooltipContent role='tooltip' className='block max-w-sm break-words'>
        <div>
          {t('Attempt started')}: {formatter.format(timing.started_at_ms)}
        </div>
        <div>
          {t('Result recorded')}: {formatter.format(timing.ended_at_ms)}
        </div>
        <div>{formatter.resolvedOptions().timeZone}</div>
      </TooltipContent>
    </Tooltip>
  )
}

function getOutcomePresentation(outcome: string) {
  switch (outcome) {
    case 'succeeded':
      return {
        label: 'Success',
        icon: CheckCircle2,
        iconClass: 'text-emerald-600',
        circleClass: 'border-emerald-200 bg-emerald-50',
      }
    case 'retrying_channel':
      return {
        label: 'Retry',
        icon: RefreshCw,
        iconClass: 'text-rose-600',
        circleClass: 'border-rose-200 bg-rose-50',
      }
    case 'channel_exhausted':
      return {
        label: 'Exhausted',
        icon: XCircle,
        iconClass: 'text-rose-600',
        circleClass: 'border-rose-200 bg-rose-50',
      }
    case 'stopped':
      return {
        label: 'Stopped',
        icon: CircleStop,
        iconClass: 'text-amber-600',
        circleClass: 'border-amber-200 bg-amber-50',
      }
    default:
      return {
        label: outcome || 'Stopped',
        icon: XCircle,
        iconClass: 'text-rose-600',
        circleClass: 'border-rose-200 bg-rose-50',
      }
  }
}

const reasonTranslationKeys: Record<string, string> = {
  channel_error: 'Channel error',
  invalid_precommit_upstream_response: 'Invalid upstream response',
  network_error: 'Network error',
  upstream_timeout: 'Upstream timeout',
  retryable_upstream_status: 'Retryable upstream status',
  non_retryable_status: 'Non-retryable status',
  client_aborted: 'Client cancelled',
  client_cancelled: 'Client cancelled',
  request_deadline: 'Request deadline',
  local_or_client_error: 'Local or client error',
  response_committed: 'Response already started',
  explicit_skip: 'Explicit retry disabled',
  configured_skip: 'Configured retry disabled',
  successful_status: 'Successful status',
  attempts_exhausted: 'Attempts exhausted',
}

function formatReason(
  reason: string | undefined,
  t: (key: string) => string
): string | undefined {
  if (!reason) return undefined
  const translationKey = reasonTranslationKeys[reason]
  if (translationKey) return t(translationKey)
  return reason.replaceAll('_', ' ')
}

function isValidNumber(value: number | undefined): value is number {
  return typeof value === 'number' && Number.isFinite(value)
}

export function RouteHistoryTimeline({
  attempts,
  fallbackChannelName,
  showChannelNames = true,
  compact = false,
}: RouteHistoryTimelineProps) {
  const { t } = useTranslation()
  if (!attempts.length) return null
  const distinctChannelIds = new Set(
    attempts
      .map((attempt) => attempt.channel_id)
      .filter((channelId) => channelId > 0)
  )
  const canUseFallbackChannelName = distinctChannelIds.size <= 1

  return (
    <div
      className={cn(
        'min-w-0',
        compact ? 'max-h-80 overflow-y-auto pr-1' : 'space-y-1'
      )}
      data-testid='route-history-timeline'
    >
      {attempts.map((attempt, index) => {
        const presentation = getOutcomePresentation(attempt.outcome)
        const Icon = presentation.icon
        const isLast = index === attempts.length - 1
        let channelName = `#${attempt.channel_id}`
        if (showChannelNames) {
          const configuredName = attempt.channel_name?.trim()
          if (configuredName) {
            channelName = configuredName
          } else if (
            fallbackChannelName &&
            attempt.channel_id > 0 &&
            canUseFallbackChannelName
          ) {
            channelName = fallbackChannelName
          }
        }
        const reason = formatReason(attempt.reason, t)
        let statusCode: number | undefined
        if (isValidNumber(attempt.status_code) && attempt.status_code > 0) {
          statusCode = attempt.status_code
        }
        const hasPriority = isValidNumber(attempt.priority)
        const hasWeight = isValidNumber(attempt.weight)
        let statusClass = 'border-rose-500 text-rose-600'
        if (statusCode != null && statusCode >= 200 && statusCode < 300) {
          statusClass = 'border-emerald-500 text-emerald-600'
        } else if (statusCode === 499) {
          statusClass = 'border-amber-500 text-amber-700'
        }
        let outcomeClass = 'border-rose-500 text-rose-600'
        if (presentation.label === 'Success') {
          outcomeClass = 'border-emerald-500 text-emerald-600'
        } else if (presentation.label === 'Stopped') {
          outcomeClass = 'border-amber-500 text-amber-700'
        }

        return (
          <div
            key={`${attempt.channel_id}-${attempt.attempt}-${attempt.outcome}-${attempt.status_code ?? ''}-${attempt.reason ?? ''}`}
            className={cn('relative flex min-w-0 gap-2', compact && 'gap-1.5')}
          >
            <div className='flex shrink-0 flex-col items-center'>
              <span
                className={cn(
                  'flex size-6 items-center justify-center rounded-full border',
                  compact && 'size-5',
                  presentation.circleClass
                )}
              >
                <Icon
                  className={cn(
                    'size-3.5',
                    compact && 'size-3',
                    presentation.iconClass
                  )}
                  aria-hidden='true'
                />
              </span>
              {!isLast && <span className='bg-border min-h-3 w-px flex-1' />}
            </div>

            <div className={cn('min-w-0 flex-1', isLast ? 'pb-1' : 'pb-3')}>
              <div className='flex min-w-0 flex-wrap items-center gap-x-1.5 gap-y-1'>
                <span className='min-w-0 text-xs font-medium break-words'>
                  {channelName}
                </span>
                {showChannelNames && attempt.channel_id > 0 && (
                  <span className='text-muted-foreground font-mono text-[10px]'>
                    #{attempt.channel_id}
                  </span>
                )}
                <Badge
                  variant='outline'
                  className={cn('px-1.5 py-0 text-[10px]', outcomeClass)}
                >
                  {t(presentation.label)}
                </Badge>
                {statusCode != null && (
                  <Badge
                    variant='outline'
                    className={cn(
                      'px-1.5 py-0 font-mono text-[10px]',
                      statusClass
                    )}
                  >
                    HTTP {statusCode}
                  </Badge>
                )}
              </div>

              <div className='text-muted-foreground mt-0.5 flex min-w-0 flex-wrap items-center gap-x-2 gap-y-0.5 text-[10px]'>
                <span>
                  {t('Attempt {{number}}', { number: attempt.attempt })}
                </span>
                {hasPriority && (
                  <span>
                    {t('Priority')} {attempt.priority}
                  </span>
                )}
                {hasWeight && (
                  <span>
                    {t('Weight')} {attempt.weight}
                  </span>
                )}
              </div>

              <AttemptTiming attempt={attempt} />
              {reason && (
                <div
                  className='text-muted-foreground mt-0.5 min-w-0 text-[10px] break-words'
                  title={attempt.reason}
                >
                  {t('Reason')}: {reason}
                </div>
              )}
            </div>
          </div>
        )
      })}
    </div>
  )
}
