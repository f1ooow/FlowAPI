/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/

import type { GroupMonitoringState } from '../types'

/**
 * The backend decides healthy / degraded / down / no-data from the raw
 * counters. The frontend must not re-derive a state from `success_count` and
 * `request_count`, or the two would drift apart; it only maps state to colour.
 */
const BUCKET_CLASSES: Record<GroupMonitoringState, string> = {
  healthy: 'bg-emerald-500 dark:bg-emerald-400',
  degraded: 'bg-amber-500 dark:bg-amber-400',
  down: 'bg-red-500 dark:bg-red-400',
  'no-data': 'bg-muted',
}

const TEXT_CLASSES: Record<GroupMonitoringState, string> = {
  healthy: 'text-emerald-600 dark:text-emerald-400',
  degraded: 'text-amber-600 dark:text-amber-400',
  down: 'text-red-600 dark:text-red-400',
  'no-data': 'text-muted-foreground',
}

const STATE_LABELS: Record<GroupMonitoringState, string> = {
  healthy: 'Operational',
  degraded: 'Degraded',
  down: 'Outage',
  'no-data': 'No data',
}

export function bucketClassName(state: GroupMonitoringState) {
  return BUCKET_CLASSES[state] ?? BUCKET_CLASSES['no-data']
}

export function stateTextClassName(state: GroupMonitoringState) {
  return TEXT_CLASSES[state] ?? TEXT_CLASSES['no-data']
}

/** Returns the i18n key for a state; callers render it through `t()`. */
export function stateLabelKey(state: GroupMonitoringState) {
  return STATE_LABELS[state] ?? STATE_LABELS['no-data']
}
