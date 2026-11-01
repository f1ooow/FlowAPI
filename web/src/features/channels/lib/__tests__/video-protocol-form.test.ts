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

import { channelSchema } from '../../types'
import {
  CHANNEL_FORM_DEFAULT_VALUES,
  channelFormSchema,
  transformChannelToFormDefaults,
  transformFormDataToCreatePayload,
} from '../channel-form'
import { hasAdvancedSettingsErrors } from '../channel-form-errors'

describe('video protocol form', () => {
  test.each(['wan-rolldek-v1', 'rolldek-kling4'] as const)(
    'preserves %s when an existing channel is edited and saved',
    (protocol) => {
      const channel = channelSchema.parse({
        id: 1,
        type: 55,
        key: '',
        status: 1,
        name: 'Rolldek video',
        created_time: 0,
        test_time: 0,
        response_time: 0,
        balance_updated_time: 0,
        used_quota: 0,
        settings: JSON.stringify({ video_protocol: protocol }),
      })

      const values = transformChannelToFormDefaults(channel)
      const payload = transformFormDataToCreatePayload(values)

      expect(values.video_protocol).toBe(protocol)
      expect(JSON.parse(String(payload.channel.settings)).video_protocol).toBe(
        protocol
      )
    }
  )

  test('accepts the Kling 4.0 profile and rejects unknown profiles', () => {
    const values = {
      ...CHANNEL_FORM_DEFAULT_VALUES,
      type: 55,
      name: 'Rolldek Kling 4.0',
      key: 'test-key',
      models: 'kling-4.0',
      video_protocol: 'rolldek-kling4',
    }

    expect(channelFormSchema.safeParse(values).success).toBe(true)
    expect(
      channelFormSchema.safeParse({ ...values, video_protocol: 'unknown' })
        .success
    ).toBe(false)
  })

  test('clearing a profile removes it without losing other settings', () => {
    const payload = transformFormDataToCreatePayload({
      ...CHANNEL_FORM_DEFAULT_VALUES,
      settings: '{"video_protocol":"rolldek-kling4","existing_setting":true}',
      video_protocol: '',
    })
    const settings = JSON.parse(String(payload.channel.settings))
    expect(settings).not.toHaveProperty('video_protocol')
    expect(settings.existing_setting).toBe(true)
  })

  test.each([1, 55, 50, 60])(
    'limits Kling 4.0 profile for channel type %s',
    (type) => {
      const result = channelFormSchema.safeParse({
        ...CHANNEL_FORM_DEFAULT_VALUES,
        type,
        name: 'Kling video',
        base_url: 'https://provider.example',
        key: 'test-key',
        models: 'kling-4.0',
        video_protocol: 'rolldek-kling4',
      })
      expect(result.success).toBe([1, 55].includes(type))
      if (!result.success) {
        expect(result.error.issues).toEqual(
          expect.arrayContaining([
            expect.objectContaining({ path: ['video_protocol'] }),
          ])
        )
        expect(
          hasAdvancedSettingsErrors({
            video_protocol: { message: 'Invalid profile' },
          })
        ).toBe(true)
      }
    }
  )
})
