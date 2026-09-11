/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/

/**
 * Health state of a timeline bucket or of a model over the whole window.
 * The backend owns the thresholds; the frontend only maps a state to colour.
 */
export type GroupMonitoringState = 'healthy' | 'degraded' | 'down' | 'no-data'

/** Display bucket widths offered by the timeline, in minutes. */
export const GROUP_MONITORING_BUCKET_MINUTE_OPTIONS = [5, 15, 30, 60] as const

export type GroupMonitoringBucketMinutes =
  (typeof GROUP_MONITORING_BUCKET_MINUTE_OPTIONS)[number]

export type GroupMonitoringBucket = {
  /** Unix seconds of the bucket start. */
  ts: number
  request_count: number
  success_count: number
  state: GroupMonitoringState
}

export type GroupMonitoringModelSummary = {
  model_name: string
  has_data: boolean
  state: GroupMonitoringState
  /** Percentage in [0, 100]. Only meaningful when `has_data` is true. */
  availability_rate: number
  /** Average total request latency. Never a time-to-first-token value. */
  avg_latency_ms: number
  /** `null` when the model produced no time-to-first-token samples. */
  avg_ttft_ms: number | null
  ttft_sample_count: number
  request_count: number
  buckets: GroupMonitoringBucket[]
}

export type GroupMonitoringGroupSummary = {
  group_name: string
  description: string
  models: GroupMonitoringModelSummary[]
}

export type GroupMonitoringSummary = {
  enabled: boolean
  /** Effective bucket width; may be wider than configured when storage is coarser. */
  bucket_minutes: number
  window_hours: number
  groups: GroupMonitoringGroupSummary[]
}

export type GroupMonitoringGroupConfig = {
  group: string
  description: string
  models: string[]
}

export type GroupMonitoringSetting = {
  enabled: boolean
  bucket_minutes: number
  groups: GroupMonitoringGroupConfig[]
}

export type GroupMonitoringAdminData = {
  setting: GroupMonitoringSetting
  available_groups: string[]
  available_models_by_group: Record<string, string[]>
  /** Physical perf_metrics bucket width; finer display buckets cannot be served. */
  storage_bucket_minutes: number
}

export type ApiResponse<T> = {
  success: boolean
  data: T
  message?: string
}
