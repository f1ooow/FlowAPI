/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/

import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { cn } from '@/lib/utils'

import {
  bucketClassName,
  stateDotClassName,
  stateLabelKey,
  stateTextClassName,
} from '../lib/status'
import type {
  GroupMonitoringBucket,
  GroupMonitoringModelSummary,
} from '../types'

function formatClockTime(unixSeconds: number) {
  return new Intl.DateTimeFormat(undefined, {
    hour: '2-digit',
    minute: '2-digit',
  }).format(unixSeconds * 1000)
}

function formatMilliseconds(
  value: number,
  translate: (key: string, options: Record<string, unknown>) => string
) {
  if (value >= 10_000) {
    return translate('{{value}} s', { value: (value / 1000).toFixed(1) })
  }
  return translate('{{value}} ms', { value: Math.round(value) })
}

function bucketTitle(
  bucket: GroupMonitoringBucket,
  translate: (key: string, options?: Record<string, unknown>) => string
) {
  const time = formatClockTime(bucket.ts)
  if (bucket.state === 'no-data') {
    return `${time} · ${translate('No data')}`
  }
  const rate = Math.round((bucket.success_count / bucket.request_count) * 100)
  return `${time} · ${rate}% (${bucket.success_count}/${bucket.request_count})`
}

/**
 * One monitored model inside a group section: 24h uptime, a latency figure and
 * the bucket timeline. The bar count is driven entirely by `model.buckets`,
 * which the backend sizes from the configured bucket width.
 */
export function ModelStatusCard(props: {
  model: GroupMonitoringModelSummary
  bucketMinutes: number
  windowHours: number
}) {
  const { t } = useTranslation()
  const hasData = props.model.has_data
  const avgTtftMs = props.model.avg_ttft_ms

  const availability = hasData
    ? `${props.model.availability_rate.toFixed(2)}%`
    : '--'

  // A model that never streams (image generation, embeddings, rerank) has no
  // time-to-first-token samples at all, which the API reports as a null average
  // rather than 0 ms. Falling back to the average total latency is fine, but it
  // must never be labelled as first-token latency.
  let latencyLabel = t('Latency (24H)')
  let latencyValue = '--'
  let latencyHint = t('No requests in the last 24 hours')
  if (avgTtftMs !== null) {
    latencyLabel = t('First token (24H)')
    latencyValue = formatMilliseconds(avgTtftMs, t)
    latencyHint = t(
      'Average time to first token over {{count}} streamed requests',
      { count: props.model.ttft_sample_count }
    )
  } else if (hasData) {
    latencyValue = formatMilliseconds(props.model.avg_latency_ms, t)
    latencyHint = t(
      'Average total request latency. No time to first token samples.'
    )
  }

  return (
    <Card size='sm' className='rounded-lg'>
      <CardHeader className='grid grid-cols-[minmax(0,1fr)_auto] items-center gap-3'>
        <CardTitle className='flex min-w-0 items-center gap-2'>
          <span
            className={cn(
              'size-2 shrink-0 rounded-full',
              stateDotClassName(props.model.state)
            )}
            aria-hidden='true'
          />
          <span className='truncate font-mono' title={props.model.model_name}>
            {props.model.model_name}
          </span>
        </CardTitle>
        <Badge
          variant='outline'
          data-state={props.model.state}
          className={cn('shrink-0', stateTextClassName(props.model.state))}
        >
          {t(stateLabelKey(props.model.state))}
        </Badge>
      </CardHeader>
      <CardContent className='space-y-3'>
        <div className='flex flex-wrap items-baseline gap-x-8 gap-y-2'>
          <div>
            <div
              className={cn(
                'font-mono text-xl leading-none font-semibold',
                stateTextClassName(props.model.state)
              )}
            >
              {availability}
            </div>
            <div className='text-muted-foreground mt-1 text-[11px] uppercase'>
              {t('Uptime (24H)')}
            </div>
          </div>
          <div>
            <div
              className='font-mono text-xl leading-none font-semibold'
              title={latencyHint}
            >
              {latencyValue}
            </div>
            <div className='text-muted-foreground mt-1 text-[11px] uppercase'>
              {latencyLabel}
            </div>
          </div>
          <div>
            <div className='font-mono text-xl leading-none font-semibold'>
              {props.model.request_count.toLocaleString()}
            </div>
            <div className='text-muted-foreground mt-1 text-[11px] uppercase'>
              {t('Requests (24H)')}
            </div>
          </div>
        </div>

        <div
          className='grid h-6 auto-cols-fr grid-flow-col gap-0 sm:gap-px'
          role='img'
          aria-label={t('Availability timeline for the last {{hours}} hours', {
            hours: props.windowHours,
          })}
        >
          {props.model.buckets.map((bucket) => (
            <span
              key={bucket.ts}
              data-state={bucket.state}
              className={cn(
                'min-w-0 rounded-[1px]',
                bucketClassName(bucket.state)
              )}
              title={bucketTitle(bucket, t)}
            />
          ))}
        </div>

        <div className='text-muted-foreground flex items-center justify-between gap-2 text-[11px]'>
          <span>{t('{{hours}}h ago', { hours: props.windowHours })}</span>
          <span>
            {t('{{minutes}} min per bar', { minutes: props.bucketMinutes })}
          </span>
          <span>{t('Now')}</span>
        </div>
      </CardContent>
    </Card>
  )
}
