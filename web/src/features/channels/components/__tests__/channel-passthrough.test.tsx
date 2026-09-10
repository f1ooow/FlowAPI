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
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { channelSchema, type Channel } from '../../types'
import { ChannelsProvider } from '../channels-provider'
import { ChannelMutateDrawer } from '../drawers/channel-mutate-drawer'

// The brand icon package uses directory imports unsupported by Node's ESM loader.
vi.mock('@lobehub/icons', () => new Proxy({}, { has: () => true }))

function existingChannel(localBody: boolean) {
  return channelSchema.parse({
    id: 42,
    type: 1,
    name: 'Upstream',
    key: '',
    status: 1,
    created_time: 0,
    test_time: 0,
    response_time: 0,
    balance_updated_time: 0,
    models: 'gpt-4o',
    setting: JSON.stringify({ pass_through_body_enabled: localBody }),
    header_override: '{"X-Upstream":"channel-value"}',
  })
}

function renderDrawer(currentRow?: Channel) {
  let globallyEnabled = true
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/channel/passthrough') {
      return {
        data: {
          success: true,
          data: {
            pass_through_request_enabled: globallyEnabled,
            pass_through_headers_enabled: globallyEnabled,
          },
        },
      }
    }
    if (url === '/api/channel/42') {
      return { data: { success: true, data: currentRow } }
    }
    return { data: { success: true, data: [] } }
  })
  useAuthStore.getState().auth.setUser({
    id: 1,
    username: 'root',
    role: ROLE.SUPER_ADMIN,
  })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  const view = render(
    <QueryClientProvider client={client}>
      <ChannelsProvider>
        <ChannelMutateDrawer
          open
          onOpenChange={() => undefined}
          currentRow={currentRow}
        />
      </ChannelsProvider>
    </QueryClientProvider>
  )
  return {
    async disableGlobalPassthrough() {
      globallyEnabled = false
      await act(async () => {
        await client.invalidateQueries({ queryKey: ['channel-passthrough'] })
      })
    },
    cleanup() {
      view.unmount()
      client.clear()
    },
  }
}

beforeEach(() => {
  const storage = new Map<string, string>()
  vi.stubGlobal('localStorage', {
    getItem: (key: string) => storage.get(key) ?? null,
    setItem: (key: string, value: string) => storage.set(key, value),
    removeItem: (key: string) => storage.delete(key),
  })
})

afterEach(() => {
  useAuthStore.getState().auth.reset()
  vi.unstubAllGlobals()
})

describe('channel passthrough inheritance', () => {
  test.each([
    { name: 'new channel', channel: undefined, localBody: false },
    {
      name: 'existing channel with local passthrough off',
      channel: existingChannel(false),
      localBody: false,
    },
    {
      name: 'existing channel with local passthrough on',
      channel: existingChannel(true),
      localBody: true,
    },
  ])(
    '$name restores its local body setting when global passthrough is disabled',
    async ({ channel, localBody }) => {
      const user = userEvent.setup()
      const view = renderDrawer(channel)
      try {
        if (!channel) {
          await user.click(
            screen.getByRole('button', {
              name: /Advanced Settings/,
              expanded: false,
            })
          )
        }
        await screen.findByText('Request body passthrough is enabled globally')
        expect(
          screen.getByText('Request header passthrough is enabled globally')
        ).toBeVisible()
        const bodySwitch = screen.getByRole('switch', {
          name: 'Pass Through Body',
        })
        expect(bodySwitch).toBeChecked()
        expect(bodySwitch).toHaveAttribute('aria-disabled', 'true')
        await user.click(bodySwitch)

        await view.disableGlobalPassthrough()

        await waitFor(() =>
          expect(bodySwitch).not.toHaveAttribute('aria-disabled', 'true')
        )
        if (localBody) {
          expect(bodySwitch).toBeChecked()
        } else {
          expect(bodySwitch).not.toBeChecked()
        }
        expect(
          screen.queryByText('Request header passthrough is enabled globally')
        ).not.toBeInTheDocument()
        await user.click(bodySwitch)
        if (localBody) {
          expect(bodySwitch).not.toBeChecked()
        } else {
          expect(bodySwitch).toBeChecked()
        }
      } finally {
        view.cleanup()
      }
    }
  )

  test('saving an inherited channel preserves the stored local body and header settings', async () => {
    const user = userEvent.setup()
    const request = vi
      .spyOn(api, 'put')
      .mockResolvedValue({ data: { success: true } })
    const view = renderDrawer(existingChannel(false))
    try {
      await screen.findByText('Request body passthrough is enabled globally')
      await user.click(screen.getByRole('button', { name: 'Update Channel' }))
      await waitFor(() => expect(request).toHaveBeenCalled())
      const payload = request.mock.calls[0][1] as Channel
      expect(
        JSON.parse(payload.setting ?? '{}').pass_through_body_enabled
      ).toBe(false)
      expect(JSON.parse(payload.header_override ?? '{}')).toEqual({
        'X-Upstream': 'channel-value',
      })
    } finally {
      view.cleanup()
    }
  })
})
