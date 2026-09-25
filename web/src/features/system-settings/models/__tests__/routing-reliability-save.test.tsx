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
import { RoutingReliabilitySection } from '../routing-reliability-section'

const defaults = {
  RetryTimes: 0,
  ChannelDisableThreshold: '',
  AutomaticDisableChannelEnabled: false,
  AutomaticEnableChannelEnabled: false,
  AutomaticDisableKeywords: '',
  AutomaticDisableStatusCodes: '401',
  AutomaticRetryStatusCodes: '429,500-599',
  'monitor_setting.auto_test_channel_enabled': false,
  'monitor_setting.auto_test_channel_minutes': 10,
  'monitor_setting.channel_test_concurrency': 1,
  'monitor_setting.channel_test_mode': 'scheduled_all' as const,
}

describe('RoutingReliabilitySection save', () => {
  test('keeps a rejected change pending so the next save retries it', async () => {
    const user = userEvent.setup()
    const request = vi
      .spyOn(api, 'put')
      .mockResolvedValueOnce({ data: { success: false, message: 'Rejected' } })
      .mockResolvedValue({ data: { success: true } })
    const client = new QueryClient({
      defaultOptions: { mutations: { retry: false } },
    })
    const actions = document.createElement('div')
    document.body.appendChild(actions)
    const view = render(
      <QueryClientProvider client={client}>
        <SettingsPageProvider actionsContainer={actions}>
          <RoutingReliabilitySection defaultValues={defaults} />
        </SettingsPageProvider>
      </QueryClientProvider>
    )
    try {
      await user.click(
        screen.getByRole('switch', { name: 'Re-enable on success' })
      )
      await user.click(screen.getByRole('button', { name: 'Save Changes' }))
      await waitFor(() => expect(request).toHaveBeenCalledTimes(1))
      await waitFor(() =>
        expect(
          screen.getByRole('button', { name: 'Save Changes' })
        ).toBeEnabled()
      )

      await user.click(screen.getByRole('button', { name: 'Save Changes' }))

      await waitFor(() => expect(request).toHaveBeenCalledTimes(2))
      expect(request).toHaveBeenLastCalledWith('/api/option/', {
        key: 'AutomaticEnableChannelEnabled',
        value: true,
      })
    } finally {
      view.unmount()
      client.clear()
      actions.remove()
      request.mockRestore()
    }
  })
})
