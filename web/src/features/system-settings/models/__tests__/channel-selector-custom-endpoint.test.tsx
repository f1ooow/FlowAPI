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
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { describe, expect, test, vi } from 'vitest'

import type { UpstreamChannel } from '../../types'
import { ChannelSelectorDialog } from '../channel-selector-dialog'

// `t` must keep a stable identity like the real hook does: the column
// definitions are memoized on it, and a fresh function per render would
// rebuild them and remount every cell, masking the focus regression below.
const translate = (key: string) => key
vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: translate }),
}))

const channel: UpstreamChannel = {
  id: 7,
  name: 'Acme upstream',
  base_url: 'https://acme.example.com',
  status: 1,
}

/**
 * Mirrors how `UpstreamRatioSync` owns the endpoint map: the dialog is a
 * controlled component, so the "custom" selection only survives a re-render if
 * the parent state round-trip preserves it.
 */
function DialogHarness() {
  const [channelEndpoints, setChannelEndpoints] = useState<
    Record<number, string>
  >({ [channel.id]: '/api/pricing' })

  return (
    <ChannelSelectorDialog
      open
      onOpenChange={() => {}}
      channels={[channel]}
      selectedChannelIds={[]}
      onSelectedChannelIdsChange={() => {}}
      channelEndpoints={channelEndpoints}
      onChannelEndpointsChange={setChannelEndpoints}
      onConfirm={() => {}}
    />
  )
}

describe('ChannelSelectorDialog sync endpoint', () => {
  test('keeps the custom option selected and reveals the endpoint input after choosing custom', async () => {
    const user = userEvent.setup()
    render(<DialogHarness />)

    const trigger = screen.getByRole('combobox', { name: 'Sync Endpoint' })
    expect(trigger).toHaveTextContent('pricing')

    await user.click(trigger)
    await user.click(screen.getByRole('option', { name: 'custom' }))

    expect(
      screen.getByRole('combobox', { name: 'Sync Endpoint' })
    ).toHaveTextContent('custom')
    expect(
      screen.getByRole('textbox', { name: 'Custom sync endpoint' })
    ).toHaveValue('')
  })

  test('preserves a typed custom endpoint instead of snapping back to the pricing preset', async () => {
    const user = userEvent.setup()
    render(<DialogHarness />)

    await user.click(screen.getByRole('combobox', { name: 'Sync Endpoint' }))
    await user.click(screen.getByRole('option', { name: 'custom' }))
    await user.type(
      screen.getByRole('textbox', { name: 'Custom sync endpoint' }),
      '/my/prices'
    )

    expect(
      screen.getByRole('textbox', { name: 'Custom sync endpoint' })
    ).toHaveValue('/my/prices')
    expect(
      screen.getByRole('combobox', { name: 'Sync Endpoint' })
    ).toHaveTextContent('custom')
  })

  test('returns to a preset endpoint and hides the custom input when a preset is picked again', async () => {
    const user = userEvent.setup()
    render(<DialogHarness />)

    await user.click(screen.getByRole('combobox', { name: 'Sync Endpoint' }))
    await user.click(screen.getByRole('option', { name: 'custom' }))
    await user.click(screen.getByRole('combobox', { name: 'Sync Endpoint' }))
    await user.click(screen.getByRole('option', { name: 'ratio_config' }))

    expect(
      screen.getByRole('combobox', { name: 'Sync Endpoint' })
    ).toHaveTextContent('ratio_config')
    expect(
      screen.queryByRole('textbox', { name: 'Custom sync endpoint' })
    ).not.toBeInTheDocument()
  })
})
