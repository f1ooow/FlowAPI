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
import { useFormContext } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import {
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { safeNumberFieldProps } from '@/features/system-settings/utils/numeric-field'

import type { ChannelFormValues } from '../../../lib/channel-form'

type ReliabilityFieldName =
  | 'channel_max_attempts'
  | 'auto_ban_threshold'
  | 'auto_ban_duration_minutes'

type ReliabilityField = {
  name: ReliabilityFieldName
  label: string
  description: string
  min: number
  max: number
}

const RELIABILITY_FIELDS: ReliabilityField[] = [
  {
    name: 'channel_max_attempts',
    label: 'Maximum attempts per channel',
    description: 'Includes the first request before switching channels.',
    min: 1,
    max: 10,
  },
  {
    name: 'auto_ban_threshold',
    label: 'Automatic ban threshold',
    description:
      'Qualifying failures before this channel is automatically banned.',
    min: 1,
    max: 100,
  },
  {
    name: 'auto_ban_duration_minutes',
    label: 'Automatic ban duration (minutes)',
    description:
      'How long an automatic ban lasts before the channel is restored.',
    min: 1,
    max: 1440,
  },
]

const TIMEOUT_FIELDS = [
  {
    name: 'first_content_timeout_seconds',
    label: 'Streaming first-content threshold (seconds)',
    description:
      '0: racing disabled. Range: 1-180 seconds. Loser billing follows the global routing reliability setting.',
    max: 180,
  },
  {
    name: 'streaming_idle_timeout_seconds',
    label: 'Streaming idle timeout (seconds)',
    description:
      '0: channel idle timer disabled. Custom range: 60-600 seconds.',
    max: 600,
  },
  {
    name: 'non_streaming_timeout_seconds',
    label: 'Non-streaming total timeout (seconds)',
    description:
      '0: added timer disabled. Custom range: 60-1800 seconds. Transport limits still apply.',
    max: 1800,
  },
] as const

export function ChannelReliabilityFields() {
  const { t } = useTranslation()
  const form = useFormContext<ChannelFormValues>()

  return (
    <div className='space-y-3'>
      <div className='space-y-1'>
        <h4 className='text-sm font-medium'>{t('Channel ban policy')}</h4>
        <p className='text-muted-foreground text-xs'>
          {t(
            'Retry this channel before switching, then automatically ban it after repeated qualifying failures.'
          )}
        </p>
      </div>
      <div className='grid gap-4 sm:grid-cols-2'>
        {RELIABILITY_FIELDS.map((config) => (
          <FormField
            key={config.name}
            control={form.control}
            name={config.name}
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t(config.label)}</FormLabel>
                <FormControl>
                  <Input
                    type='number'
                    min={config.min}
                    max={config.max}
                    step={1}
                    {...safeNumberFieldProps(field)}
                  />
                </FormControl>
                <FormDescription>{t(config.description)}</FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />
        ))}
      </div>
      <div className='space-y-3 border-t pt-4'>
        <h4 className='text-sm font-medium'>{t('Request timeouts')}</h4>
        <div className='grid gap-4 sm:grid-cols-2'>
          {TIMEOUT_FIELDS.map((config) => (
            <FormField
              key={config.name}
              control={form.control}
              name={config.name}
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t(config.label)}</FormLabel>
                  <FormControl>
                    <Input
                      type='number'
                      min={0}
                      max={config.max}
                      step={1}
                      {...safeNumberFieldProps(field)}
                      onChange={(event) => {
                        if (event.target.value === '') {
                          field.onChange(0)
                        } else if (
                          Number.isFinite(event.target.valueAsNumber)
                        ) {
                          field.onChange(event.target.valueAsNumber)
                        }
                      }}
                    />
                  </FormControl>
                  <FormDescription>{t(config.description)}</FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
          ))}
        </div>
      </div>
    </div>
  )
}
