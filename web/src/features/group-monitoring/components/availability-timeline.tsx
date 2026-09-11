/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/

import { useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { cn } from '@/lib/utils'

import { bucketClassName } from '../lib/status'
import {
  mergeTimelineBuckets,
  type GroupMonitoringTimelineBar,
} from '../lib/timeline'
import type { GroupMonitoringBucket, GroupMonitoringThresholds } from '../types'

/**
 * Horizontal space one bar needs to stay readable, gap included. A 24 hour
 * window at the finest bucket is 288 buckets, which would be sub-pixel inside a
 * card, so bars are merged until they fit.
 */
const MIN_BAR_SLOT_PX = 4

/**
 * Used until the container has been measured, and in environments without a
 * working ResizeObserver. Roughly a 280px wide card, which is the narrowest
 * card the grid produces.
 */
const UNMEASURED_BAR_CAPACITY = 70

function formatClockTime(unixSeconds: number) {
  return new Intl.DateTimeFormat(undefined, {
    hour: '2-digit',
    minute: '2-digit',
  }).format(unixSeconds * 1000)
}

/**
 * The bucket width used to live in a caption line under every card. It is in
 * the hover title instead, as the covered time range of the bar.
 */
function barTitle(
  bar: GroupMonitoringTimelineBar,
  translate: (key: string, options?: Record<string, unknown>) => string
) {
  const range = `${formatClockTime(bar.ts)} – ${formatClockTime(bar.end_ts)}`
  if (bar.request_count <= 0) {
    return `${range} · ${translate('No data')}`
  }
  const rate = Math.round((bar.success_count / bar.request_count) * 100)
  return `${range} · ${rate}% (${bar.success_count}/${bar.request_count})`
}

export function AvailabilityTimeline(props: {
  buckets: GroupMonitoringBucket[]
  /** Width of one returned bucket, in minutes. */
  bucketMinutes: number
  windowHours: number
  thresholds: GroupMonitoringThresholds
}) {
  const { t } = useTranslation()
  const containerRef = useRef<HTMLDivElement>(null)
  const [width, setWidth] = useState(0)

  useEffect(() => {
    const element = containerRef.current
    if (!element) return
    setWidth(element.getBoundingClientRect().width)
    const observer = new ResizeObserver((entries) => {
      setWidth(entries[0]?.contentRect.width ?? 0)
    })
    observer.observe(element)
    return () => observer.disconnect()
  }, [])

  const capacity =
    width > 0
      ? Math.max(1, Math.floor(width / MIN_BAR_SLOT_PX))
      : UNMEASURED_BAR_CAPACITY

  const bars = useMemo(
    () =>
      mergeTimelineBuckets(
        props.buckets,
        capacity,
        props.bucketMinutes * 60,
        props.thresholds
      ),
    [props.buckets, capacity, props.bucketMinutes, props.thresholds]
  )

  return (
    <div
      ref={containerRef}
      role='img'
      aria-label={t('Availability timeline for the last {{hours}} hours', {
        hours: props.windowHours,
      })}
      className='grid h-7 auto-cols-fr grid-flow-col gap-px'
    >
      {bars.map((bar) => (
        <span
          key={bar.ts}
          data-state={bar.state}
          className={cn('min-w-0 rounded-[2px]', bucketClassName(bar.state))}
          title={barTitle(bar, t)}
        />
      ))}
    </div>
  )
}
