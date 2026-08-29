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
import { getApiKeys } from '@/features/keys/api'

import type { PlaygroundToken } from '../types'

export async function getPlaygroundTokens(): Promise<PlaygroundToken[]> {
  const tokens: PlaygroundToken[] = []
  let page = 1
  let total = 0

  do {
    const response = await getApiKeys({ p: page, size: 100 })
    if (!response.success) {
      throw new Error(response.message || 'Unable to load API keys')
    }
    const items = response.data?.items || []
    tokens.push(
      ...items.map((token) => ({
        id: token.id,
        name: token.name,
        maskedKey: token.key,
        status: token.status,
      }))
    )
    total = response.data?.total || tokens.length
    page += 1
    if (!items.length) break
  } while (tokens.length < total)

  return tokens
}
