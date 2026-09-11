/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/

import { render, screen } from '@testing-library/react'
import { describe, expect, test } from 'vitest'

import type {
  GroupMonitoringBucket,
  GroupMonitoringModelSummary,
} from '../../types'
import { ModelStatusCard } from '../model-status-card'

function bucket(
  ts: number,
  request_count: number,
  success_count: number,
  state: GroupMonitoringBucket['state']
): GroupMonitoringBucket {
  return { ts, request_count, success_count, state }
}

const streamingModel: GroupMonitoringModelSummary = {
  model_name: 'claude-sonnet-4-6',
  has_data: true,
  state: 'healthy',
  availability_rate: 99.53,
  avg_latency_ms: 8400,
  avg_ttft_ms: 820,
  ttft_sample_count: 1180,
  request_count: 1204,
  buckets: [
    bucket(0, 0, 0, 'no-data'),
    bucket(300, 10, 10, 'healthy'),
    bucket(600, 10, 9, 'degraded'),
    bucket(900, 10, 2, 'down'),
  ],
}

describe('ModelStatusCard', () => {
  test('labels the metric as first token latency when TTFT samples exist', () => {
    render(
      <ModelStatusCard
        model={streamingModel}
        bucketMinutes={5}
        windowHours={24}
      />
    )

    expect(screen.getByText('First token (24H)')).toBeInTheDocument()
    expect(screen.getByText('820 ms')).toBeInTheDocument()
    expect(screen.queryByText('Latency (24H)')).toBeNull()
    expect(screen.getByText('99.53%')).toBeInTheDocument()
  })

  test('falls back to total latency under a distinct label when TTFT is null', () => {
    render(
      <ModelStatusCard
        model={{
          ...streamingModel,
          model_name: 'gpt-image-2',
          avg_ttft_ms: null,
          ttft_sample_count: 0,
          avg_latency_ms: 117260,
        }}
        bucketMinutes={5}
        windowHours={24}
      />
    )

    expect(screen.getByText('Latency (24H)')).toBeInTheDocument()
    expect(screen.getByText('117.3 s')).toBeInTheDocument()
    expect(screen.queryByText('First token (24H)')).toBeNull()
    expect(
      screen.getByTitle(/No time to first token samples/)
    ).toBeInTheDocument()
  })

  test('shows a real zero TTFT instead of the no-sample fallback', () => {
    render(
      <ModelStatusCard
        model={{ ...streamingModel, avg_ttft_ms: 0, ttft_sample_count: 42 }}
        bucketMinutes={5}
        windowHours={24}
      />
    )

    expect(screen.getByText('First token (24H)')).toBeInTheDocument()
    expect(screen.getByText('0 ms')).toBeInTheDocument()
  })

  test('renders one timeline bar per returned bucket with the backend state', () => {
    const view = render(
      <ModelStatusCard
        model={streamingModel}
        bucketMinutes={5}
        windowHours={24}
      />
    )

    const bars = [...view.container.querySelectorAll('[data-state]')]
    expect(bars).toHaveLength(streamingModel.buckets.length + 1) // + status badge
    const timeline = view.container.querySelector('[role="img"]')
    expect(timeline?.children).toHaveLength(4)
    expect(
      [...(timeline?.children ?? [])].map((element) =>
        element.getAttribute('data-state')
      )
    ).toEqual(['no-data', 'healthy', 'degraded', 'down'])
  })

  test('renders a no-data model without an availability number', () => {
    render(
      <ModelStatusCard
        model={{
          model_name: 'sora-2',
          has_data: false,
          state: 'no-data',
          availability_rate: 0,
          avg_latency_ms: 0,
          avg_ttft_ms: null,
          ttft_sample_count: 0,
          request_count: 0,
          buckets: [bucket(0, 0, 0, 'no-data')],
        }}
        bucketMinutes={60}
        windowHours={24}
      />
    )

    expect(screen.getAllByText('--')).toHaveLength(2)
    expect(screen.queryByText('0.00%')).toBeNull()
    expect(screen.getByText('No data')).toBeInTheDocument()
  })
})
