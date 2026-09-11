/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/

import { render, screen } from '@testing-library/react'
import { describe, expect, test, vi } from 'vitest'

import type {
  GroupMonitoringBucket,
  GroupMonitoringModelSummary,
  GroupMonitoringThresholds,
} from '../../types'
import { ModelStatusCard } from '../model-status-card'

// The brand icon package uses directory imports unsupported by Node's ESM loader.
vi.mock('@lobehub/icons', () => new Proxy({}, { has: () => true }))

const THRESHOLDS: GroupMonitoringThresholds = {
  healthy_rate: 99,
  degraded_rate: 80,
  min_bucket_requests: 3,
}

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
  icon: 'Claude',
  vendor_name: 'Anthropic',
  vendor_icon: 'Anthropic.Color',
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

function renderCard(model: GroupMonitoringModelSummary, bucketMinutes = 5) {
  return render(
    <ModelStatusCard
      model={model}
      bucketMinutes={bucketMinutes}
      windowHours={24}
      thresholds={THRESHOLDS}
    />
  )
}

describe('ModelStatusCard', () => {
  test('labels the metric as TTFT when time-to-first-token samples exist', () => {
    renderCard(streamingModel)

    expect(screen.getByText('TTFT (1H)')).toBeInTheDocument()
    expect(screen.getByText('820 ms')).toBeInTheDocument()
    expect(screen.queryByText('Latency (1H)')).toBeNull()
    expect(screen.getByText('99.53%')).toBeInTheDocument()
  })

  test('shows the vendor name under the model name', () => {
    renderCard(streamingModel)

    expect(screen.getByText('claude-sonnet-4-6')).toBeInTheDocument()
    expect(screen.getByText('Anthropic')).toBeInTheDocument()
  })

  test('falls back to a placeholder provider when the model has no vendor', () => {
    renderCard({
      ...streamingModel,
      icon: undefined,
      vendor_name: undefined,
      vendor_icon: undefined,
    })

    expect(screen.getByText('Unknown provider')).toBeInTheDocument()
  })

  test('falls back to total latency under a distinct label when TTFT is null', () => {
    renderCard({
      ...streamingModel,
      model_name: 'gpt-image-2',
      avg_ttft_ms: null,
      ttft_sample_count: 0,
      avg_latency_ms: 117260,
    })

    expect(screen.getByText('Latency (1H)')).toBeInTheDocument()
    expect(screen.getByText('117.3 s')).toBeInTheDocument()
    expect(screen.queryByText('TTFT (1H)')).toBeNull()
    expect(screen.getByTitle(/No streamed samples/)).toBeInTheDocument()
  })

  test('shows no latency for a model that served nothing in the last hour', () => {
    renderCard({
      ...streamingModel,
      // Plenty of traffic today, none of it inside the latency window.
      avg_latency_ms: null,
      avg_ttft_ms: null,
      ttft_sample_count: 0,
    })

    expect(screen.getByText('Latency (1H)')).toBeInTheDocument()
    // The 24h availability keeps its value, only the latency is unknown.
    expect(screen.getByText('99.53%')).toBeInTheDocument()
    expect(screen.getAllByText('—')).toHaveLength(1)
    expect(
      screen.getByTitle('No requests in the last hour')
    ).toBeInTheDocument()
  })

  test('shows a real zero TTFT instead of the no-sample fallback', () => {
    renderCard({ ...streamingModel, avg_ttft_ms: 0, ttft_sample_count: 42 })

    expect(screen.getByText('TTFT (1H)')).toBeInTheDocument()
    expect(screen.getByText('0 ms')).toBeInTheDocument()
  })

  test('renders one timeline bar per bucket while they fit the card', () => {
    const view = renderCard(streamingModel)

    const timeline = view.container.querySelector('[role="img"]')
    expect(timeline?.children).toHaveLength(streamingModel.buckets.length)
    expect(
      [...(timeline?.children ?? [])].map((element) =>
        element.getAttribute('data-state')
      )
    ).toEqual(['no-data', 'healthy', 'degraded', 'down'])
  })

  test('collapses a full 24h series so every bar keeps a visible width', () => {
    const buckets = Array.from({ length: 288 }, (_, index) =>
      bucket(index * 300, 10, 10, 'healthy')
    )
    const view = renderCard({ ...streamingModel, buckets })

    const timeline = view.container.querySelector('[role="img"]')
    // Merged down from 288 buckets: sub-pixel bars are unreadable in a card.
    expect(timeline?.children.length).toBeLessThan(buckets.length)
    expect(timeline?.children.length).toBeGreaterThan(0)
  })

  test('renders a no-data model without an availability number', () => {
    renderCard(
      {
        model_name: 'sora-2',
        has_data: false,
        state: 'no-data',
        availability_rate: 0,
        avg_latency_ms: null,
        avg_ttft_ms: null,
        ttft_sample_count: 0,
        request_count: 0,
        buckets: [bucket(0, 0, 0, 'no-data')],
      },
      60
    )

    expect(screen.getAllByText('—')).toHaveLength(2)
    expect(screen.queryByText('0.00%')).toBeNull()
    expect(screen.getByText('No data')).toBeInTheDocument()
    // A model with no traffic at all must not claim the hour is the problem.
    expect(
      screen.getByTitle('No requests in the last 24 hours')
    ).toBeInTheDocument()
  })
})
