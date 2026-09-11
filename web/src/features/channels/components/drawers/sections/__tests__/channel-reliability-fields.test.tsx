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
import { FormProvider, useForm } from 'react-hook-form'
import { describe, expect, test, vi } from 'vitest'

import { Form } from '@/components/ui/form'

import type { ChannelFormValues } from '../../../../lib/channel-form'
import { ChannelReliabilityFields } from '../channel-reliability-fields'

const translate = (key: string) => key
vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: translate }),
}))

function ReliabilityHarness() {
  const form = useForm<ChannelFormValues>({
    defaultValues: {
      channel_max_attempts: 2,
      auto_ban_threshold: 5,
      auto_ban_duration_minutes: 30,
    } as ChannelFormValues,
  })
  const attempts = form.watch('channel_max_attempts')

  return (
    <FormProvider {...form}>
      <Form {...form}>
        <ChannelReliabilityFields />
        <output data-testid='attempts-state'>{String(attempts)}</output>
      </Form>
    </FormProvider>
  )
}

describe('ChannelReliabilityFields', () => {
  test('never renders NaN or stores NaN when the attempts field is cleared', async () => {
    const user = userEvent.setup()
    render(<ReliabilityHarness />)

    const input = screen.getByLabelText(
      'Maximum attempts per channel'
    ) as HTMLInputElement
    expect(input).toHaveValue(2)

    await user.clear(input)

    expect(input.value).not.toBe('NaN')
    expect(screen.getByTestId('attempts-state')).toHaveTextContent('2')
  })

  test('commits a replacement number to form state when the attempts field is overwritten', async () => {
    const user = userEvent.setup()
    render(<ReliabilityHarness />)

    const input = screen.getByLabelText('Maximum attempts per channel')
    await user.tripleClick(input)
    await user.keyboard('7')

    expect(input).toHaveValue(7)
    expect(screen.getByTestId('attempts-state')).toHaveTextContent('7')
  })
})
