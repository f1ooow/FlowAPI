/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/

import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, within } from '@testing-library/react'
import { afterEach, describe, expect, test, vi } from 'vitest'

import { useAuthStore } from '@/stores/auth-store'

import { getGroupMonitoringSummary } from '../api'
import { GroupMonitoring } from '../index'
import type { GroupMonitoringSummary } from '../types'

vi.mock('@lobehub/icons', () => new Proxy({}, { has: () => true }))

vi.mock('../api', () => ({
  getGroupMonitoringSummary: vi.fn(),
}))

const INITIAL_AUTH_STATE = useAuthStore.getState()

const SUMMARY: GroupMonitoringSummary = {
  enabled: true,
  bucket_minutes: 5,
  window_hours: 24,
  thresholds: {
    healthy_rate: 99,
    degraded_rate: 80,
    min_bucket_requests: 3,
  },
  groups: [
    {
      id: 'monitor-1',
      group_name: 'North Pool',
      description: '',
      visible_to_groups: null,
      models: [],
      request_count: 100,
      success_count: 100,
      availability_rate: 100,
    },
    {
      id: 'monitor-2',
      group_name: 'South Pool',
      description: '',
      visible_to_groups: null,
      models: [],
      request_count: 50,
      success_count: 49,
      availability_rate: 98,
    },
  ],
  range: '24h',
  ranges: ['24h', '72h'],
  updated_at: 1_800_000_000,
  overall: {
    request_count: 150,
    success_count: 149,
    availability_rate: 99.33,
  },
}

afterEach(() => {
  useAuthStore.setState(INITIAL_AUTH_STATE)
})

describe('GroupMonitoring layout', () => {
  test('renders each monitor as a full-width row instead of a multi-column card', async () => {
    vi.mocked(getGroupMonitoringSummary).mockResolvedValue(SUMMARY)
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })

    render(
      <QueryClientProvider client={queryClient}>
        <GroupMonitoring />
      </QueryClientProvider>
    )

    const monitors = await screen.findByRole('list', {
      name: 'Group Monitoring',
    })
    expect(monitors).toHaveClass('grid-cols-1')
    expect(within(monitors).getAllByRole('listitem')).toHaveLength(2)
    expect(screen.queryByText('Overall availability')).toBeNull()
    expect(screen.queryByText(/Statistics through/)).toBeNull()
    queryClient.clear()
  })
})
