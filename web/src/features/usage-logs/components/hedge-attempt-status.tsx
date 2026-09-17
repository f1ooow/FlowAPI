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
import { useTranslation } from 'react-i18next'

import { StatusBadge } from '@/components/status-badge'

import type { LogOtherData } from '../types'

export function HedgeAttemptStatus(props: {
  hedge: NonNullable<LogOtherData['hedge']>
}) {
  const { t } = useTranslation()

  return (
    <StatusBadge
      label={
        props.hedge.role === 'winner'
          ? t('Racing winner')
          : t('Extra racing attempt')
      }
      variant={props.hedge.role === 'winner' ? 'info' : 'warning'}
      size='sm'
      copyable={false}
    />
  )
}

export function HedgeMeteringStatus(props: {
  hedge: NonNullable<LogOtherData['hedge']>
}) {
  const { t } = useTranslation()
  let label = t('Metered usage')
  if (props.hedge.usage_source === 'estimated') label = t('Estimated usage')
  if (props.hedge.metering_status === 'partial') label = t('Partial usage')
  if (props.hedge.metering_status === 'unmetered') label = t('Usage unknown')

  return <span className='text-muted-foreground text-xs'>{label}</span>
}
