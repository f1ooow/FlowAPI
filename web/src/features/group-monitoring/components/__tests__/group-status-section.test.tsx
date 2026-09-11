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
    request_count: 100,
    buckets: [
      { ts: 0, request_count: 10, success_count: 10, state: 'healthy' },
    ],
  }
}

const GROUP: GroupMonitoringGroupSummary = {
  group_name: 'default',
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
      thresholds={THRESHOLDS}
    />
  )
}

describe('GroupStatusSection', () => {
  test('shows the group name, description and model count in the header', () => {
    renderSection()

    expect(screen.getByRole('heading', { name: 'default' })).toBeInTheDocument()
    expect(screen.getByText('shared pool')).toBeInTheDocument()
    expect(screen.getByText('2 models')).toBeInTheDocument()
  })

  test('counts degraded and down models as issues', () => {
    renderSection()

    expect(screen.getByText('1 with issues')).toBeInTheDocument()
  })

  test('hides the issue badge when every model is healthy', () => {
    renderSection({ ...GROUP, models: [model('gpt-4o-mini', 'healthy')] })

    expect(screen.queryByText(/with issues/)).toBeNull()
  })

  test('collapses and restores the model cards from the header toggle', async () => {
    const user = userEvent.setup()
    renderSection()

    const toggle = screen.getByRole('button', { name: 'Toggle default' })
    expect(screen.getByText('gpt-4o-mini')).toBeInTheDocument()

    await user.click(toggle)
    expect(screen.queryByText('gpt-4o-mini')).toBeNull()

    await user.click(toggle)
    expect(screen.getByText('gpt-4o-mini')).toBeInTheDocument()
  })

  test('marks a group restricted to an allow list', () => {
    renderSection({ ...GROUP, visible_to_groups: ['vip', 'internal'] })

    expect(screen.getByText('Visible to vip, internal')).toBeInTheDocument()
  })

  test('marks a group with an empty allow list as administrators only', () => {
    renderSection({ ...GROUP, visible_to_groups: [] })

    expect(screen.getByText('Administrators only')).toBeInTheDocument()
  })

  test('leaves an unrestricted group unmarked', () => {
    renderSection()

    expect(screen.queryByText(/Visible to|Administrators only/)).toBeNull()
  })

  test('explains an empty group instead of rendering a blank grid', () => {
    renderSection({ ...GROUP, models: [] })

    expect(
      screen.getByText('No models configured for this group')
    ).toBeInTheDocument()
  })
})
