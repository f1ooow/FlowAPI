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
type Translate = (key: string, options?: Record<string, unknown>) => string

/**
 * Timeline slots can be two hours wide over a 7 day range, so a bucket tooltip
 * carries the day as well as the clock time.
 */
export function formatBucketTime(unixSeconds: number) {
  return new Intl.DateTimeFormat(undefined, {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  }).format(unixSeconds * 1000)
}

export function formatMilliseconds(value: number, translate: Translate) {
  if (value >= 10_000) {
    return translate('{{value}} s', { value: (value / 1000).toFixed(1) })
  }
  return translate('{{value}} ms', { value: Math.round(value) })
}
