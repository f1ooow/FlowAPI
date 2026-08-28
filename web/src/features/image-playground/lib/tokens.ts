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
