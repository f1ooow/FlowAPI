/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/

import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { Card, CardContent, CardHeader } from '@/components/ui/card'
import { getLobeIcon } from '@/lib/lobe-icon'
import { cn } from '@/lib/utils'

import { stateLabelKey, stateTextClassName } from '../lib/status'
import type {
  GroupMonitoringModelSummary,
  GroupMonitoringThresholds,
} from '../types'
import { AvailabilityTimeline } from './availability-timeline'

/** Shown wherever a metric has no sample to report. */
const NO_VALUE = '—'

function formatMilliseconds(
  value: number,
  translate: (key: string, options: Record<string, unknown>) => string
) {
  if (value >= 10_000) {
    return translate('{{value}} s', { value: (value / 1000).toFixed(1) })
  }
  return translate('{{value}} ms', { value: Math.round(value) })
}

/**
 * One monitored model: identity on top, the two headline metrics in the middle
 * and the availability timeline at the bottom.
 */
export function ModelStatusCard(props: {
  model: GroupMonitoringModelSummary
  bucketMinutes: number
  windowHours: number
  thresholds: GroupMonitoringThresholds
}) {
  const { t } = useTranslation()
  const hasData = props.model.has_data
  const avgTtftMs = props.model.avg_ttft_ms
  const avgLatencyMs = props.model.avg_latency_ms

  const availability = hasData
    ? `${props.model.availability_rate.toFixed(2)}%`
    : NO_VALUE

  // Availability covers 24 hours, latency only the last one: a day-long mean
  // says nothing about how the model feels right now. The two are deliberately
  // on different windows, so the labels must say which one they are on.
  //
  // A model that never streams (image generation, embeddings, rerank) has no
  // time-to-first-token samples at all, which the API reports as a null average
  // rather than 0 ms. Falling back to the average total latency is fine, but it
  // must never be labelled as first-token latency.
  let latencyLabel = t('Latency (1H)')
  let latencyValue = NO_VALUE
  let latencyHint = hasData
    ? t('No requests in the last hour')
    : t('No requests in the last 24 hours')
  if (avgTtftMs !== null) {
    latencyLabel = t('TTFT (1H)')
    latencyValue = formatMilliseconds(avgTtftMs, t)
    latencyHint = t(
      'Average time to first token over {{count}} streamed requests in the last hour',
      { count: props.model.ttft_sample_count }
    )
  } else if (avgLatencyMs !== null) {
    latencyValue = formatMilliseconds(avgLatencyMs, t)
    latencyHint = t(
      'Average total request latency over the last hour. No streamed samples in that window.'
    )
  }

  // The model icon and its vendor fall back to each other so a model without
  // its own artwork still shows the vendor's.
  const iconKey = props.model.icon || props.model.vendor_icon

  return (
    <Card size='sm' className='rounded-xl'>
      <CardHeader className='grid grid-cols-[auto_minmax(0,1fr)_auto] items-start gap-2'>
        <span
          className='bg-muted/50 flex size-8 shrink-0 items-center justify-center rounded-lg'
          aria-hidden='true'
        >
          {getLobeIcon(iconKey, 18)}
        </span>
        <span className='min-w-0'>
          <span
            className='block truncate font-mono text-sm font-medium'
            title={props.model.model_name}
          >
            {props.model.model_name}
          </span>
          <span className='text-muted-foreground block truncate text-xs'>
            {props.model.vendor_name || t('Unknown provider')}
          </span>
        </span>
        <Badge
          variant='outline'
          data-state={props.model.state}
          className={cn('shrink-0', stateTextClassName(props.model.state))}
        >
          {t(stateLabelKey(props.model.state))}
        </Badge>
      </CardHeader>
      <CardContent className='space-y-3'>
        <div className='grid grid-cols-2 gap-2'>
          <div
            className='rounded-lg border px-2.5 py-2'
            title={t('{{count}} requests in the last {{hours}} hours', {
              count: props.model.request_count,
              hours: props.windowHours,
            })}
          >
            <div className='text-muted-foreground truncate text-[11px]'>
              {t('Uptime (24H)')}
            </div>
            <div
              className={cn(
                'mt-1 font-mono text-lg leading-none font-semibold',
                stateTextClassName(props.model.state)
              )}
            >
              {availability}
            </div>
          </div>
          <div className='rounded-lg border px-2.5 py-2' title={latencyHint}>
            <div className='text-muted-foreground truncate text-[11px]'>
              {latencyLabel}
            </div>
            <div className='mt-1 font-mono text-lg leading-none font-semibold'>
              {latencyValue}
            </div>
          </div>
        </div>

        <AvailabilityTimeline
          buckets={props.model.buckets}
          bucketMinutes={props.bucketMinutes}
          windowHours={props.windowHours}
          thresholds={props.thresholds}
        />
      </CardContent>
    </Card>
  )
}
