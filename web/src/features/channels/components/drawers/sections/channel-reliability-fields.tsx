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
    </div>
  )
}
