/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/

import type {
  GroupMonitoringBucket,
  GroupMonitoringState,
  GroupMonitoringThresholds,
} from '../types'

/** A rendered timeline bar, built from one or more consecutive buckets. */
export type GroupMonitoringTimelineBar = {
  /** Unix seconds of the first covered bucket. */
  ts: number
  /** Exclusive unix seconds end of the last covered bucket. */
  end_ts: number
  request_count: number
  success_count: number
  /** How many source buckets this bar covers. */
  span: number
  state: GroupMonitoringState
}

/**
 * Colours a bar from its summed counters with the thresholds the backend
 * reported. This is deliberately the same rule the backend applies to a single
 * display bucket, so a merged bar shows exactly what a wider configured bucket
 * would have shown.
 */
function barState(
  requestCount: number,
  successCount: number,
  thresholds: GroupMonitoringThresholds
): GroupMonitoringState {
  if (requestCount < thresholds.min_bucket_requests || requestCount <= 0) {
    return 'no-data'
  }
  // Rounded to two decimals before the comparison, matching the backend, so a
  // rate sitting exactly on a threshold lands on the same side in both places.
  const rate = Math.round((successCount / requestCount) * 100 * 100) / 100
  if (rate >= thresholds.healthy_rate) return 'healthy'
  if (rate >= thresholds.degraded_rate) return 'degraded'
  return 'down'
}

/**
 * Collapses the returned buckets into at most `capacity` bars so every bar
 * keeps a visible width inside a narrow card.
 *
 * Counters are summed and the rate derived once per bar. Averaging the
 * per-bucket rates would give a bucket with a single request the same weight as
 * one with a thousand.
 */
export function mergeTimelineBuckets(
  buckets: GroupMonitoringBucket[],
  capacity: number,
  bucketSeconds: number,
  thresholds: GroupMonitoringThresholds
): GroupMonitoringTimelineBar[] {
  if (buckets.length === 0) return []

  const step = Math.max(1, Math.ceil(buckets.length / Math.max(1, capacity)))
  const bars: GroupMonitoringTimelineBar[] = []

  for (let index = 0; index < buckets.length; index += step) {
    const slice = buckets.slice(index, index + step)
    let requestCount = 0
    let successCount = 0
    for (const bucket of slice) {
      requestCount += bucket.request_count
      successCount += bucket.success_count
    }
    bars.push({
      ts: slice[0].ts,
      end_ts: slice[0].ts + slice.length * bucketSeconds,
      request_count: requestCount,
      success_count: successCount,
      span: slice.length,
      state: barState(requestCount, successCount, thresholds),
    })
  }

  return bars
}
