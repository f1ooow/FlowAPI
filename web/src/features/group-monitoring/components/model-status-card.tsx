/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/

import { ChartNoAxesCombined } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { getLobeIcon } from '@/lib/lobe-icon'
import { cn } from '@/lib/utils'

import { stateLabelKey, stateTextClassName } from '../lib/status'
import type {
  GroupMonitoringModelSummary,
  GroupMonitoringThresholds,
} from '../types'
import { AvailabilityTimeline } from './availability-timeline'

export function ModelStatusCard(props: {
  model: GroupMonitoringModelSummary
  bucketMinutes: number
  windowHours: number
  thresholds: GroupMonitoringThresholds
  onOpenDetails: (model: GroupMonitoringModelSummary) => void
}) {
  const { t } = useTranslation()
  const ttft = props.model.avg_ttft_ms_window
  const latency = props.model.avg_latency_ms
  let timingLabel = t('TTFT')
  let timingValue = '—'
  let timingHint = t('No streamed samples in this window')
  if (ttft !== null) {
    timingValue = t('{{value}} ms', {
      value: Math.round(ttft).toLocaleString(),
    })
    timingHint = t(
      'Mean time to first token over {{count}} streamed requests in {{hours}} hours',
      { count: props.model.ttft_sample_count_window, hours: props.windowHours }
    )
  } else if (latency !== null) {
    timingLabel = t('Total latency (1H)')
    timingValue = t('{{value}} ms', {
      value: Math.round(latency).toLocaleString(),
    })
    timingHint = t(
      'No streamed samples in this window. Showing mean total request latency over the last hour, not TTFT.'
    )
  }

  return (
    <div className='min-w-0 space-y-3 py-4'>
      <div className='grid min-w-0 gap-3 lg:grid-cols-[minmax(0,1fr)_minmax(28rem,38rem)] lg:items-center'>
        <div className='flex min-w-0 items-center gap-2.5'>
          <span
            className='bg-muted/50 flex size-8 shrink-0 items-center justify-center rounded-lg'
            aria-hidden='true'
          >
            {getLobeIcon(props.model.icon || props.model.vendor_icon, 18)}
          </span>
          <div className='min-w-0 flex-1'>
            <button
              type='button'
              className='hover:text-primary focus-visible:ring-ring inline-flex min-h-9 max-w-full cursor-pointer items-center gap-2 rounded text-left focus-visible:ring-2 focus-visible:outline-none'
              aria-label={t('View trends for {{model}}', {
                model: props.model.model_name,
              })}
              onClick={() => props.onOpenDetails(props.model)}
            >
              <span className='min-w-0 font-mono text-sm font-semibold [overflow-wrap:anywhere]'>
                {props.model.model_name}
              </span>
              <ChartNoAxesCombined
                className='text-muted-foreground size-4 shrink-0'
                aria-hidden='true'
              />
            </button>
            <p className='text-muted-foreground text-xs'>
              {props.model.vendor_name || t('Unknown provider')}
            </p>
          </div>
          <Badge
            variant='outline'
            className={cn(
              'shrink-0 lg:hidden',
              stateTextClassName(props.model.state)
            )}
          >
            {t(stateLabelKey(props.model.state))}
          </Badge>
        </div>
        <dl className='grid min-w-0 grid-cols-2 gap-x-3 gap-y-2 text-sm sm:grid-cols-4 lg:text-right'>
          <div>
            <dt className='text-muted-foreground text-xs lg:sr-only'>
              {t('Success rate')}
            </dt>
            <dd
              className={cn(
                'font-semibold tabular-nums',
                stateTextClassName(props.model.state)
              )}
            >
              {props.model.has_data
                ? `${props.model.availability_rate.toFixed(2)}%`
                : '—'}
            </dd>
          </div>
          <div>
            <dt className='text-muted-foreground text-xs lg:sr-only'>
              {t('Requests')}
            </dt>
            <dd className='tabular-nums'>
              {props.model.request_count.toLocaleString()}
            </dd>
          </div>
          <div title={timingHint}>
            <dt className='text-muted-foreground text-xs lg:sr-only'>
              {timingLabel}
            </dt>
            <dd className='tabular-nums'>{timingValue}</dd>
            {ttft === null && latency !== null && (
              <span className='text-muted-foreground text-[11px]'>
                {timingLabel}
              </span>
            )}
          </div>
          <div>
            <dt className='text-muted-foreground text-xs lg:sr-only'>
              {t('Output speed')}
            </dt>
            <dd className='tabular-nums'>
              {props.model.avg_output_tps === null
                ? '—'
                : t('{{value}} token/s', {
                    value: props.model.avg_output_tps.toFixed(1),
                  })}
            </dd>
          </div>
        </dl>
      </div>
      <AvailabilityTimeline
        buckets={props.model.buckets}
        bucketMinutes={props.bucketMinutes}
        windowHours={props.windowHours}
        thresholds={props.thresholds}
      />
    </div>
  )
}
