/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/

import { describe, expect, test } from 'vitest'

import type {
  GroupMonitoringBucket,
  GroupMonitoringState,
  GroupMonitoringThresholds,
} from '../../types'
import { mergeTimelineBuckets } from '../timeline'

const BUCKET_SECONDS = 300

// Mirrors what the API reports today; the merge logic reads them from the
// payload instead of hard-coding a second copy.
const THRESHOLDS: GroupMonitoringThresholds = {
  healthy_rate: 99,
  degraded_rate: 80,
  min_bucket_requests: 3,
}

function bucket(
  index: number,
  request_count: number,
  success_count: number,
  state: GroupMonitoringState
): GroupMonitoringBucket {
  return {
    ts: index * BUCKET_SECONDS,
    request_count,
    success_count,
    state,
  }
}

describe('mergeTimelineBuckets', () => {
  test('returns nothing for an empty series', () => {
    expect(mergeTimelineBuckets([], 60, BUCKET_SECONDS, THRESHOLDS)).toEqual([])
  })

  test('keeps every bucket when the series already fits the capacity', () => {
    const buckets = [bucket(0, 10, 10, 'healthy'), bucket(1, 10, 9, 'degraded')]

    const bars = mergeTimelineBuckets(buckets, 60, BUCKET_SECONDS, THRESHOLDS)

    expect(bars).toHaveLength(2)
    expect(bars[0]).toEqual({
      ts: 0,
      end_ts: BUCKET_SECONDS,
      request_count: 10,
      success_count: 10,
      span: 1,
      state: 'healthy',
    })
  })

  test('reproduces the backend state for every unmerged bucket', () => {
    const buckets = [
      bucket(0, 0, 0, 'no-data'),
      bucket(1, 2, 2, 'no-data'),
      bucket(2, 100, 100, 'healthy'),
      bucket(3, 100, 98, 'degraded'),
      bucket(4, 100, 79, 'down'),
      bucket(5, 100, 99, 'healthy'),
      bucket(6, 100, 80, 'degraded'),
    ]

    const bars = mergeTimelineBuckets(buckets, 60, BUCKET_SECONDS, THRESHOLDS)

    expect(bars.map((bar) => bar.state)).toEqual(
      buckets.map((sourceBucket) => sourceBucket.state)
    )
  })

  test('sums the counters of merged buckets instead of averaging their rates', () => {
    // Averaging 100%, 0% and 100% would read as 66.7% and paint the bar red.
    // Summing gives 19/22 = 86.36%, which is degraded.
    const buckets = [
      bucket(0, 10, 10, 'healthy'),
      bucket(1, 2, 0, 'no-data'),
      bucket(2, 10, 9, 'degraded'),
    ]

    const bars = mergeTimelineBuckets(buckets, 1, BUCKET_SECONDS, THRESHOLDS)

    expect(bars).toEqual([
      {
        ts: 0,
        end_ts: 3 * BUCKET_SECONDS,
        request_count: 22,
        success_count: 19,
        span: 3,
        state: 'degraded',
      },
    ])
  })

  test('lifts buckets below the sample threshold above it once merged', () => {
    const buckets = [bucket(0, 2, 2, 'no-data'), bucket(1, 2, 2, 'no-data')]

    const bars = mergeTimelineBuckets(buckets, 1, BUCKET_SECONDS, THRESHOLDS)

    expect(bars).toHaveLength(1)
    expect(bars[0].request_count).toBe(4)
    expect(bars[0].state).toBe('healthy')
  })

  test('keeps a merged bar with too few requests as no data', () => {
    const buckets = [bucket(0, 1, 0, 'no-data'), bucket(1, 1, 1, 'no-data')]

    const bars = mergeTimelineBuckets(buckets, 1, BUCKET_SECONDS, THRESHOLDS)

    expect(bars[0].state).toBe('no-data')
  })

  test('collapses a full 24h series to at most the requested capacity', () => {
    const buckets = Array.from({ length: 288 }, (_, index) =>
      bucket(index, 10, 10, 'healthy')
    )

    const bars = mergeTimelineBuckets(buckets, 70, BUCKET_SECONDS, THRESHOLDS)

    expect(bars.length).toBeLessThanOrEqual(70)
    // No request may be lost or double counted while merging.
    expect(bars.reduce((total, bar) => total + bar.request_count, 0)).toBe(2880)
    expect(bars[0].ts).toBe(0)
    expect(bars.at(-1)?.end_ts).toBe(288 * BUCKET_SECONDS)
  })

  test('covers the whole window even when the last bar is a short remainder', () => {
    const buckets = Array.from({ length: 5 }, (_, index) =>
      bucket(index, 10, 10, 'healthy')
    )

    const bars = mergeTimelineBuckets(buckets, 3, BUCKET_SECONDS, THRESHOLDS)

    expect(bars.map((bar) => bar.span)).toEqual([2, 2, 1])
    expect(bars[2].end_ts).toBe(5 * BUCKET_SECONDS)
  })
})
