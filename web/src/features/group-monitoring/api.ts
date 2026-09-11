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
  GroupMonitoringSummary,
} from './types'

/**
 * The window is fixed to 24 hours server-side, so no query parameters are sent.
 */
export async function getGroupMonitoringSummary() {
  const response = await api.get<ApiResponse<GroupMonitoringSummary>>(
    '/api/group-monitoring/summary'
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
