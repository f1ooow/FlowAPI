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
import { describe, expect, test } from 'vitest'

import {
  CHANNEL_FORM_DEFAULT_VALUES,
  channelFormSchema,
  transformFormDataToCreatePayload,
  transformFormDataToUpdatePayload,
} from '../channel-form'

describe('channel cost ratio form contract', () => {
  test('defaults new channels to a 1x cost ratio and serializes it', () => {
    expect(CHANNEL_FORM_DEFAULT_VALUES.cost_ratio).toBe(1)

    const payload = transformFormDataToCreatePayload({
      ...CHANNEL_FORM_DEFAULT_VALUES,
      name: 'channel',
      key: 'secret',
      models: 'gpt-test',
      cost_ratio: 1.5,
    })

    expect(payload.channel.cost_ratio).toBe(1.5)
  })

  test('includes cost ratio updates and rejects unsafe values', () => {
    const values = {
      ...CHANNEL_FORM_DEFAULT_VALUES,
      name: 'channel',
      key: 'secret',
      models: 'gpt-test',
      cost_ratio: 0.8,
    }

    expect(transformFormDataToUpdatePayload(values, 42).cost_ratio).toBe(0.8)
    expect(
      channelFormSchema.safeParse({ ...values, cost_ratio: 0 }).success
    ).toBe(false)
    expect(
      channelFormSchema.safeParse({ ...values, cost_ratio: 1001 }).success
    ).toBe(false)
  })
})
