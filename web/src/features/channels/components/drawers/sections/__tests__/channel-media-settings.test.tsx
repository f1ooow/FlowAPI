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
import { useForm } from 'react-hook-form'
import { describe, expect, test, vi } from 'vitest'

import { Form } from '@/components/ui/form'

import {
  CHANNEL_FORM_DEFAULT_VALUES,
  type ChannelFormValues,
} from '../../../../lib/channel-form'
import { ChannelMediaSettings } from '../channel-media-settings'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}))

function MediaHarness(props: {
  protocol?: ChannelFormValues['video_protocol']
  channelType?: number
  onSubmit: (data: ChannelFormValues) => void
}) {
  const form = useForm<ChannelFormValues>({
    defaultValues: {
      ...CHANNEL_FORM_DEFAULT_VALUES,
      video_protocol: props.protocol || '',
    },
  })
  return (
    <Form {...form}>
      <form onSubmit={form.handleSubmit(props.onSubmit)}>
        <ChannelMediaSettings channelType={props.channelType ?? 55} />
        <button type='submit'>Save</button>
      </form>
    </Form>
  )
}

describe('ChannelMediaSettings video protocol', () => {
  test('selecting Kling 4.0 explains input billing and saves its profile', async () => {
    const user = userEvent.setup()
    const onSubmit = vi.fn()
    render(<MediaHarness onSubmit={onSubmit} />)

    await user.click(screen.getByRole('combobox', { name: 'Video protocol' }))
    await user.click(screen.getByRole('option', { name: 'Rolldek Kling 4.0' }))
    expect(
      screen.getByRole('combobox', { name: 'Video protocol' })
    ).toHaveAccessibleDescription(
      'Kling 4.0 uses /v1/videos and bills output plus reference video seconds. 720p multiplier: 1; 1080p: 1.4.'
    )
    await user.click(screen.getByRole('button', { name: 'Save' }))
    expect(onSubmit.mock.calls[0]?.[0].video_protocol).toBe('rolldek-kling4')
  })

  test('an existing Wan profile can be reset to the default by keyboard', async () => {
    const user = userEvent.setup()
    const onSubmit = vi.fn()
    render(<MediaHarness protocol='wan-rolldek-v1' onSubmit={onSubmit} />)

    const select = screen.getByRole('combobox', { name: 'Video protocol' })
    expect(select).toHaveTextContent('Rolldek Wan protocol')
    select.focus()
    await user.keyboard('{Enter}{Home}{Enter}')
    expect(select).toHaveTextContent('Default')
    await user.click(screen.getByRole('button', { name: 'Save' }))
    expect(onSubmit.mock.calls[0]?.[0].video_protocol).toBe('')
  })

  test('a New API channel keeps Wan but does not offer the Kling 4.0 profile', async () => {
    const user = userEvent.setup()
    render(<MediaHarness channelType={60} onSubmit={vi.fn()} />)

    await user.click(screen.getByRole('combobox', { name: 'Video protocol' }))
    expect(
      screen.getByRole('option', { name: 'Rolldek Wan protocol' })
    ).toBeVisible()
    expect(
      screen.queryByRole('option', { name: 'Rolldek Kling 4.0' })
    ).not.toBeInTheDocument()
  })

  test('an invalid saved Kling profile explains the mismatch without silently clearing it', async () => {
    const user = userEvent.setup()
    render(
      <MediaHarness
        channelType={60}
        protocol='rolldek-kling4'
        onSubmit={vi.fn()}
      />
    )
    const select = screen.getByRole('combobox', { name: 'Video protocol' })
    expect(select).toHaveTextContent('Rolldek Kling 4.0')
    expect(select).toHaveAccessibleDescription(
      'Kling 4.0 requires an OpenAI or Sora channel.'
    )
    await user.click(select)
    expect(
      screen.getByRole('option', { name: 'Rolldek Kling 4.0' })
    ).toHaveAttribute('aria-disabled', 'true')
  })
})
