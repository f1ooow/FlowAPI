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

/**
 * Thresholds the backend applied to every bucket it returned. The timeline
 * merges adjacent buckets to fit the card width and has to colour the merged
 * bar from the summed counters, so it reuses these numbers instead of keeping
 * a second copy that could drift from the server.
 */
export type GroupMonitoringThresholds = {
  /** Availability percentage at or above which a bucket counts as healthy. */
  healthy_rate: number
  /** Availability percentage at or above which a bucket counts as degraded. */
  degraded_rate: number
  /** Below this many requests a bucket carries no signal and reads as no-data. */
  min_bucket_requests: number
}

export type GroupMonitoringModelSummary = {
  model_name: string
  /** @lobehub/icons key of the model itself, from the pricing catalog. */
  icon?: string
  vendor_name?: string
  /** @lobehub/icons key of the vendor, used when the model has no icon. */
  vendor_icon?: string
  /** Whether the model served any request in the 24 hour window. */
  has_data: boolean
  state: GroupMonitoringState
  /** Percentage in [0, 100] over 24 hours. Only meaningful when `has_data`. */
  availability_rate: number
  /**
   * Average total request latency over the last hour, never a time-to-first
   * token value. `null` when the model served no request in that hour, even if
   * it served some earlier in the day.
   */
  avg_latency_ms: number | null
  /**
   * Average time to first token over the last hour. `null` when that hour holds
   * no streamed sample, which is permanent for image/embedding/rerank models.
   */
  avg_ttft_ms: number | null
  /** Streamed samples inside the latency window, not the 24 hour window. */
  ttft_sample_count: number
  /** Requests over the 24 hour window. */
  request_count: number
  buckets: GroupMonitoringBucket[]
}

/**
 * User groups allowed to see a monitored group. The three states are distinct
 * and must not be normalised away:
 * - `null`: every logged-in user.
 * - `[]`: administrators only.
 * - non-empty: only users in the listed groups (plus administrators).
 *
 * A regular viewer always receives `null` because they only ever get groups
 * they already passed the filter for; the allow list is admin-only detail.
 */
export type GroupMonitoringVisibility = string[] | null

export type GroupMonitoringGroupSummary = {
  group_name: string
  description: string
  visible_to_groups: GroupMonitoringVisibility
  models: GroupMonitoringModelSummary[]
}

export type GroupMonitoringSummary = {
  enabled: boolean
  /** Effective bucket width; may be wider than configured when storage is coarser. */
  bucket_minutes: number
  window_hours: number
  thresholds: GroupMonitoringThresholds
  groups: GroupMonitoringGroupSummary[]
}

export type GroupMonitoringGroupConfig = {
  group: string
  description: string
  /** User-group allow list; see {@link GroupMonitoringVisibility}. */
  visible_to_groups: GroupMonitoringVisibility
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
