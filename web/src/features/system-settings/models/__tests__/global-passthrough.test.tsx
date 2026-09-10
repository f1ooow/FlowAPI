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
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import { SettingsPageProvider } from '../../components/settings-page-context'
import { GlobalSettingsCard } from '../global-settings-card'

describe('global passthrough settings', () => {
  test.each([
    {
      label: 'Enable Request Header Passthrough',
      key: 'global.pass_through_headers_enabled',
      headersInitiallyEnabled: false,
      value: true,
    },
    {
      label: 'Enable Request Body Passthrough',
      key: 'global.pass_through_request_enabled',
      headersInitiallyEnabled: true,
      value: false,
    },
  ])(
    'saves $key independently and refreshes channel inheritance',
    async ({ label, key, headersInitiallyEnabled, value }) => {
      const user = userEvent.setup()
      const request = vi.spyOn(api, 'put').mockResolvedValue({
        data: { success: true },
      })
      const client = new QueryClient({
        defaultOptions: {
          queries: { retry: false },
          mutations: { retry: false },
        },
      })
      client.setQueryData(['channel-passthrough'], {
        success: true,
        data: {
          pass_through_request_enabled: true,
          pass_through_headers_enabled: headersInitiallyEnabled,
        },
      })
      const actions = document.createElement('div')
      document.body.appendChild(actions)
      try {
        render(
          <QueryClientProvider client={client}>
            <SettingsPageProvider actionsContainer={actions}>
              <GlobalSettingsCard
                defaultValues={{
                  global: {
                    pass_through_request_enabled: true,
                    pass_through_headers_enabled: headersInitiallyEnabled,
                    thinking_model_blacklist: '[]',
                    chat_completions_to_responses_policy: '{}',
                  },
                  general_setting: {
                    ping_interval_enabled: false,
                    ping_interval_seconds: 10,
                  },
                }}
              />
            </SettingsPageProvider>
          </QueryClientProvider>
        )
        const bodySwitch = screen.getByRole('switch', {
          name: 'Enable Request Body Passthrough',
        })
        const headerSwitch = screen.getByRole('switch', {
          name: 'Enable Request Header Passthrough',
        })
        expect(bodySwitch).toBeChecked()
        if (headersInitiallyEnabled) {
          expect(headerSwitch).toBeChecked()
        } else {
          expect(headerSwitch).not.toBeChecked()
        }
        await user.click(screen.getByRole('switch', { name: label }))
        await user.click(screen.getByRole('button', { name: 'Save Changes' }))
        await waitFor(() =>
          expect(request).toHaveBeenCalledWith('/api/option/', {
            key,
            value,
          })
        )
        expect(request).toHaveBeenCalledTimes(1)
        expect(headerSwitch).toBeChecked()
        if (value) {
          expect(bodySwitch).toBeChecked()
        } else {
          expect(bodySwitch).not.toBeChecked()
        }
        await waitFor(() =>
          expect(
            client.getQueryState(['channel-passthrough'])?.isInvalidated
          ).toBe(true)
        )
      } finally {
        client.clear()
        actions.remove()
      }
    }
  )
})
