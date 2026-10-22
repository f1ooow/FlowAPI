/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/

import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, test, vi } from 'vitest'

import type {
  GroupMonitoringGroupSummary,
  GroupMonitoringModelSummary,
  GroupMonitoringThresholds,
} from '../../types'
import { GroupStatusSection } from '../group-status-section'

// The brand icon package uses directory imports unsupported by Node's ESM loader.
vi.mock('@lobehub/icons', () => new Proxy({}, { has: () => true }))

const THRESHOLDS: GroupMonitoringThresholds = {
  healthy_rate: 99,
  degraded_rate: 80,
  min_bucket_requests: 3,
}

function model(
  model_name: string,
  state: GroupMonitoringModelSummary['state']
): GroupMonitoringModelSummary {
  return {
    model_name,
    vendor_name: 'OpenAI',
    has_data: state !== 'no-data',
    state,
    availability_rate: 99.9,
    avg_latency_ms: 1200,
    avg_ttft_ms: 400,
    ttft_sample_count: 10,
    avg_ttft_ms_24h: 450,
    ttft_sample_count_24h: 240,
    avg_ttft_ms_window: 450,
    ttft_sample_count_window: 240,
    avg_output_tps: 60,
    request_count: 100,
    success_count: 99,
    buckets: [
      {
        ts: 0,
        request_count: 10,
        success_count: 10,
        state: 'healthy',
        avg_ttft_ms: 400,
        avg_latency_ms: 1200,
        avg_output_tps: 60,
      },
    ],
  }
}

const GROUP: GroupMonitoringGroupSummary = {
  id: 'monitor-1',
  group_name: 'Claude Max',
  source_groups: ['default', 'vip'],
  request_count: 200,
  success_count: 198,
  availability_rate: 99,
  description: 'shared pool',
  visible_to_groups: null,
  models: [model('gpt-4o-mini', 'healthy'), model('gpt-image-2', 'down')],
}

function renderSection(group: GroupMonitoringGroupSummary = GROUP) {
  return render(
    <GroupStatusSection
      group={group}
      bucketMinutes={5}
      windowHours={24}
      range='24h'
      thresholds={THRESHOLDS}
    />
  )
}

describe('GroupStatusSection', () => {
  test('shows the independent monitor name and description without summary badges', () => {
    renderSection()

    expect(
      screen.getByRole('heading', { name: 'Claude Max' })
    ).toBeInTheDocument()
    expect(screen.getByText('shared pool')).toBeInTheDocument()
    expect(screen.queryByText('2 models')).toBeNull()
    expect(screen.queryByText('200 requests')).toBeNull()
    expect(screen.queryByText('99.00% success')).toBeNull()
  })

  test('lays out model monitors in a dense responsive grid', () => {
    renderSection()

    const models = screen.getByRole('list')
    expect(models).toHaveClass(
      'grid',
      'sm:grid-cols-2',
      'lg:grid-cols-3',
      'xl:grid-cols-4'
    )
    expect(within(models).getAllByRole('listitem')).toHaveLength(2)
  })

  test('opens the selected model trend sheet and closes with Escape', async () => {
    const user = userEvent.setup()
    renderSection()
    await user.click(
      screen.getByRole('button', { name: 'View trends for gpt-4o-mini' })
    )
    expect(screen.getByRole('dialog')).toBeInTheDocument()
    expect(
      screen.getByRole('heading', { name: 'gpt-4o-mini' })
    ).toBeInTheDocument()
    await user.keyboard('{Escape}')
    expect(screen.queryByRole('dialog')).toBeNull()
  })

  test('does not expose source pricing groups or visibility metadata', () => {
    renderSection()
    expect(screen.queryByText('Sources: default, vip')).toBeNull()
    expect(screen.queryByText(/Visible to|Administrators only/)).toBeNull()
  })

  test('explains an empty group instead of rendering a blank grid', () => {
    renderSection({ ...GROUP, models: [] })

    expect(
      screen.getByText('No models configured for this monitor')
    ).toBeInTheDocument()
  })
})
