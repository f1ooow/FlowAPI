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
import type { UserGroupDisplay } from '@/lib/api'

export type GroupRatio =
  | { kind: 'single'; min: number; max: number }
  | { kind: 'range'; min: number; max: number }
  | { kind: 'auto' }
  | { kind: 'unavailable' }
  | null
  | undefined

function formatRatioValue(value: number): string {
  return Number(value.toFixed(4)).toString()
}

export function formatGroupRatio(
  ratio: GroupRatio,
  auto: string,
  unavailable: string
): string {
  if (!ratio) return ''
  if (ratio.kind === 'auto') return auto
  if (ratio.kind === 'unavailable') return unavailable
  if (ratio.kind === 'range' && ratio.min !== ratio.max) {
    return `${formatRatioValue(ratio.min)}x-${formatRatioValue(ratio.max)}x`
  }
  return `${formatRatioValue(ratio.min)}x`
}

export function groupRatioFromApi(info: UserGroupDisplay): GroupRatio {
  if (info.ratio_kind === 'auto') return { kind: 'auto' }
  if (info.ratio_kind === 'unavailable') return { kind: 'unavailable' }
  if (!Number.isFinite(info.ratio_min) || !Number.isFinite(info.ratio_max)) {
    return { kind: 'unavailable' }
  }
  return {
    kind: info.ratio_kind,
    min: info.ratio_min,
    max: info.ratio_max,
  }
}
