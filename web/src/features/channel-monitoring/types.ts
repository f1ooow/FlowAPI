/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
/**
 * Health state of a timeline bucket or of a channel over the whole window.
 * The backend owns the thresholds; the frontend only maps a state to colour.
 */
export type ChannelMonitoringState = 'healthy' | 'degraded' | 'down' | 'no-data'

export type ChannelMonitoringBucket = {
  /** Unix seconds of the bucket start. */
  ts: number
  /**
   * Channel attempts, not user requests: one request that fails over three
   * times contributes one attempt to each of the three channels.
   */
  attempt_count: number
  success_count: number
  state: ChannelMonitoringState
}

export type ChannelMonitoringChannelSummary = {
  channel_id: number
  /** Empty once the channel is deleted; the UI falls back to `#<id>`. */
  channel_name: string
  has_data: boolean
  state: ChannelMonitoringState
  /** Percentage in [0, 100]. Only meaningful when `has_data` is true. */
  availability_rate: number
  error_rate: number
  avg_latency_ms: number
  attempt_count: number
  success_count: number
  buckets: ChannelMonitoringBucket[]
}

export type ChannelMonitoringOverall = {
  has_data: boolean
  availability_rate: number
  error_rate: number
  avg_latency_ms: number
  attempt_count: number
}

export type ChannelMonitoringSummary = {
  /** The range the backend actually served, which may differ from the request. */
  range: string
  /** Supported ranges in display order; the range switcher is built from this. */
  ranges: string[]
  /** Effective bucket width; wider than the nominal step when storage is coarser. */
  step_minutes: number
  window_seconds: number
  overall: ChannelMonitoringOverall
  /** Pre-sorted worst-availability first, channels without data last. */
  channels: ChannelMonitoringChannelSummary[]
}

export type ApiResponse<T> = {
  success: boolean
  data: T
  message?: string
}
