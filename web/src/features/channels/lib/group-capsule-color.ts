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
 * Solid fills for group capsules. Each shade keeps white text readable in
 * both themes; the full class strings stay literal so Tailwind can find them.
 */
const GROUP_CAPSULE_CLASSES = [
  'bg-indigo-700',
  'bg-fuchsia-700',
  'bg-violet-700',
  'bg-sky-700',
  'bg-emerald-700',
  'bg-amber-600',
  'bg-rose-700',
  'bg-teal-700',
  'bg-blue-700',
  'bg-orange-700',
  'bg-cyan-700',
  'bg-pink-700',
] as const

/** Fill of a group that the group list does not know, from its name alone. */
function hashedGroupCapsuleClassName(group: string): string {
  let hash = 0x811c9dc5
  for (const char of group) {
    hash ^= char.codePointAt(0) ?? 0
    hash = Math.imul(hash, 0x01000193)
  }
  return GROUP_CAPSULE_CLASSES[(hash >>> 0) % GROUP_CAPSULE_CLASSES.length]
}

/**
 * Assigns every group of the full group list its own capsule fill: groups are
 * sorted by name and take the palette in order, so up to twelve groups never
 * share a colour and the result does not depend on the list's order. The
 * collation is pinned so every browser sorts, and so colours, the same way.
 *
 * Groups outside the list (a leftover on a channel, or any group before the
 * list has loaded) fall back to a colour hashed from the name.
 */
export function createGroupCapsuleClassName(
  groups: readonly string[]
): (group: string) => string {
  const assigned = new Map<string, string>()
  const sorted = [...new Set(groups)].sort((a, b) => a.localeCompare(b, 'en'))
  for (const [index, group] of sorted.entries()) {
    assigned.set(
      group,
      GROUP_CAPSULE_CLASSES[index % GROUP_CAPSULE_CLASSES.length]
    )
  }
  return (group) => assigned.get(group) ?? hashedGroupCapsuleClassName(group)
}
