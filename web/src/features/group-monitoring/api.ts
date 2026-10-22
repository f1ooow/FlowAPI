/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/

import { api } from '@/lib/api'

import type {
  ApiResponse,
  GroupMonitoringAdminData,
  GroupMonitoringSetting,
  GroupMonitoringRange,
  GroupMonitoringSummary,
} from './types'

/**
 * The range parameter selects the request aggregation window, not chart granularity.
 */
export async function getGroupMonitoringSummary(range: GroupMonitoringRange) {
  const response = await api.get<ApiResponse<GroupMonitoringSummary>>(
    '/api/group-monitoring/summary',
    { params: { range } }
  )
  return response.data.data
}

export async function getGroupMonitoringAdmin() {
  const response = await api.get<ApiResponse<GroupMonitoringAdminData>>(
    '/api/group-monitoring/admin'
  )
  return response.data.data
}

export async function updateGroupMonitoringAdmin(
  setting: GroupMonitoringSetting
) {
  const response = await api.put<ApiResponse<never>>(
    '/api/group-monitoring/admin',
    setting
  )
  return response.data
}
