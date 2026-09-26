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
import { QueryClient } from '@tanstack/react-query'
import { describe, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import { handleUpdateChannelGroups } from '../channel-actions'

describe('handleUpdateChannelGroups', () => {
  test('sends only the channel id and the comma-joined groups', async () => {
    const put = vi
      .spyOn(api, 'put')
      .mockResolvedValue({ data: { success: true } })

    const saved = await handleUpdateChannelGroups(42, ['default', 'vip'])

    expect(saved).toBe(true)
    expect(put).toHaveBeenCalledTimes(1)
    expect(put.mock.calls[0][0]).toBe('/api/channel/')
    // Exactly these keys: an absent key/models field is what keeps the
    // backend from overwriting them.
    expect(put.mock.calls[0][1]).toEqual({ id: 42, group: 'default,vip' })
  })

  test('refetches the channel lists before resolving', async () => {
    vi.spyOn(api, 'put').mockResolvedValue({ data: { success: true } })
    const queryClient = new QueryClient()
    const invalidate = vi.spyOn(queryClient, 'invalidateQueries')

    await handleUpdateChannelGroups(42, ['default'], queryClient)

    expect(invalidate).toHaveBeenCalledWith({
      queryKey: ['channels', 'list'],
    })
  })

  test('reports a rejected update as not saved', async () => {
    vi.spyOn(api, 'put').mockResolvedValue({
      data: { success: false, message: 'no permission' },
    })

    await expect(handleUpdateChannelGroups(42, ['vip'])).resolves.toBe(false)
  })

  test('reports a network failure as not saved', async () => {
    vi.spyOn(api, 'put').mockRejectedValue(new Error('offline'))

    await expect(handleUpdateChannelGroups(42, ['vip'])).resolves.toBe(false)
  })
})
