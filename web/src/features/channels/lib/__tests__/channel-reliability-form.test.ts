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

describe('channel reliability form', () => {
  test.each([undefined, 0, 60])(
    'loads timeout value %s with a zero default and preserves unknown settings',
    (timeout) => {
      const channel = channelSchema.parse({
        id: 1,
        type: 1,
        key: '',
        status: 1,
        name: 'primary',
        created_time: 0,
        test_time: 0,
        response_time: 0,
        balance_updated_time: 0,
        used_quota: 0,
        settings: JSON.stringify({
          existing_setting: true,
          reliability: {
            future_setting: 'preserved',
            first_content_timeout_seconds: timeout,
            streaming_idle_timeout_seconds: timeout,
            non_streaming_timeout_seconds: timeout,
          },
        }),
      })
      const values = transformChannelToFormDefaults(channel)
      const payload = transformFormDataToCreatePayload(values)
      const settings = JSON.parse(String(payload.channel.settings))

      expect(settings.existing_setting).toBe(true)
      expect(settings.reliability.future_setting).toBe('preserved')
      for (const field of [
        'first_content_timeout_seconds',
        'streaming_idle_timeout_seconds',
        'non_streaming_timeout_seconds',
      ] as const) {
        expect(values[field]).toBe(timeout ?? 0)
        expect(settings.reliability[field]).toBe(timeout ?? 0)
        expect(Object.hasOwn(settings.reliability, field)).toBe(true)
      }
    }
  )

  test('disabling a configured timeout persists zero without removing other reliability settings', () => {
    const payload = transformFormDataToCreatePayload({
      ...CHANNEL_FORM_DEFAULT_VALUES,
      settings:
        '{"reliability":{"streaming_idle_timeout_seconds":120,"future_setting":true}}',
      streaming_idle_timeout_seconds: 0,
    })
    expect(JSON.parse(String(payload.channel.settings)).reliability).toEqual({
      max_attempts: 2,
      auto_ban_threshold: 5,
      auto_ban_duration_seconds: 1800,
      first_content_timeout_seconds: 0,
      streaming_idle_timeout_seconds: 0,
      non_streaming_timeout_seconds: 0,
      future_setting: true,
    })
  })

  test.each([
    ['first_content_timeout_seconds', 1, true],
    ['first_content_timeout_seconds', 180, true],
    ['first_content_timeout_seconds', 181, false],
    ['first_content_timeout_seconds', -1, false],
    ['streaming_idle_timeout_seconds', 59, false],
    ['streaming_idle_timeout_seconds', 600, true],
    ['streaming_idle_timeout_seconds', 601, false],
    ['non_streaming_timeout_seconds', 59, false],
    ['non_streaming_timeout_seconds', 1800, true],
    ['non_streaming_timeout_seconds', 1801, false],
    ['non_streaming_timeout_seconds', 60.5, false],
  ])('validates %s=%s with result %s', (field, value, valid) => {
    const result = channelFormSchema.safeParse({
      ...CHANNEL_FORM_DEFAULT_VALUES,
      name: 'primary',
      key: 'sk-test',
      models: 'model',
      [field]: value,
    })
    expect(result.success).toBe(valid)
    if (!result.success) {
      expect(result.error.issues.some((issue) => issue.path[0] === field)).toBe(
        true
      )
      expect(
        hasAdvancedSettingsErrors({ [field]: { message: 'Invalid timeout' } })
      ).toBe(true)
    }
  })

  test('serializes channel attempts and automatic ban settings without losing other settings', () => {
    const payload = transformFormDataToCreatePayload({
      ...CHANNEL_FORM_DEFAULT_VALUES,
      name: 'primary',
      key: 'sk-test',
      models: 'claude-opus-4-6',
      settings: '{"existing_setting":true}',
      channel_max_attempts: 3,
      auto_ban_threshold: 7,
      auto_ban_duration_minutes: 45,
    })
    const settings = JSON.parse(String(payload.channel.settings))

    expect(settings.existing_setting).toBe(true)
    expect(settings.reliability).toEqual({
      max_attempts: 3,
      auto_ban_threshold: 7,
      auto_ban_duration_seconds: 2700,
      first_content_timeout_seconds: 0,
      streaming_idle_timeout_seconds: 0,
      non_streaming_timeout_seconds: 0,
    })
  })

  test('rejects reliability values outside the supported form limits', () => {
    const result = channelFormSchema.safeParse({
      ...CHANNEL_FORM_DEFAULT_VALUES,
      name: 'primary',
      key: 'sk-test',
      models: 'model',
      channel_max_attempts: 0,
    })

    expect(result.success).toBe(false)
  })

  test('loads persisted reliability seconds into channel form values', () => {
    const channel = channelSchema.parse({
      id: 1,
      type: 1,
      key: '',
      status: 1,
      name: 'primary',
      created_time: 0,
      test_time: 0,
      response_time: 0,
      balance_updated_time: 0,
      used_quota: 0,
      settings: JSON.stringify({
        reliability: {
          max_attempts: 4,
          auto_ban_threshold: 8,
          auto_ban_duration_seconds: 3600,
        },
      }),
    })

    const values = transformChannelToFormDefaults(channel)

    expect(values.channel_max_attempts).toBe(4)
    expect(values.auto_ban_threshold).toBe(8)
    expect(values.auto_ban_duration_minutes).toBe(60)
  })
})
