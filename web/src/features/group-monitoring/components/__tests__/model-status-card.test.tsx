/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/

import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, test, vi } from 'vitest'

import type {
  GroupMonitoringModelSummary,
  GroupMonitoringThresholds,
} from '../../types'
import { ModelStatusCard } from '../model-status-card'

vi.mock('@lobehub/icons', () => new Proxy({}, { has: () => true }))

const THRESHOLDS: GroupMonitoringThresholds = {
  healthy_rate: 99,
  degraded_rate: 80,
  min_bucket_requests: 3,
}
export const MODEL: GroupMonitoringModelSummary = {
  model_name: 'claude-sonnet-4-6',
  vendor_name: 'Anthropic',
  has_data: true,
  state: 'healthy',
  availability_rate: 99.53,
  avg_latency_ms: 8400,
  avg_ttft_ms: 820,
  ttft_sample_count: 1180,
  avg_ttft_ms_24h: 910,
  ttft_sample_count_24h: 12040,
  avg_ttft_ms_window: 910,
  ttft_sample_count_window: 12040,
  avg_output_tps: 61.8,
  request_count: 1204,
  success_count: 1198,
  buckets: [
    {
      ts: 300,
      request_count: 10,
      success_count: 10,
      state: 'healthy',
      avg_ttft_ms: 200,
      avg_latency_ms: 900,
      avg_output_tps: 70,
    },
  ],
}

function renderRow(model = MODEL, onOpenDetails = vi.fn(), windowHours = 24) {
  return render(
    <ModelStatusCard
      model={model}
      bucketMinutes={5}
      windowHours={windowHours}
      thresholds={THRESHOLDS}
      onOpenDetails={onOpenDetails}
    />
  )
}

describe('ModelStatusCard', () => {
  test('shows aligned request metrics for the selected window and opens model details by keyboard', async () => {
    const user = userEvent.setup()
    const open = vi.fn()
    renderRow(MODEL, open)
    expect(screen.getByText('99.53%')).toBeInTheDocument()
    expect(screen.getByText('1,204')).toBeInTheDocument()
    expect(screen.getByText('910 ms')).toBeInTheDocument()
    expect(screen.getByText('61.8 token/s')).toBeInTheDocument()
    const trigger = screen.getByRole('button', {
      name: 'View trends for claude-sonnet-4-6',
    })
    trigger.focus()
    await user.keyboard('{Enter}')
    expect(open).toHaveBeenCalledWith(MODEL)
  })
})
