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
import { Alert02Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useId } from 'react'
import { useTranslation } from 'react-i18next'

import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { formatQuota } from '@/lib/format'

type UserCostCellProps = {
  quota: number | null
  incomplete: boolean
}

export function UserCostCell(props: UserCostCellProps) {
  const { t } = useTranslation()
  const descriptionId = useId()
  const unavailable = props.quota === null

  return (
    <Tooltip>
      <TooltipTrigger
        aria-describedby={descriptionId}
        aria-label={unavailable ? t('Cost unavailable') : undefined}
        className='focus-visible:ring-ring/50 inline-flex min-h-6 cursor-help items-center gap-1.5 rounded-sm text-sm tabular-nums outline-none focus-visible:ring-[3px]'
      >
        <span>{props.quota === null ? '-' : formatQuota(props.quota)}</span>
        {props.incomplete && !unavailable && (
          <>
            <HugeiconsIcon
              icon={Alert02Icon}
              size={14}
              className='text-warning shrink-0'
              aria-hidden='true'
            />
            <span className='sr-only'>{t('Incomplete cost history')}</span>
          </>
        )}
      </TooltipTrigger>
      <TooltipContent id={descriptionId} role='tooltip'>
        <div className='space-y-1'>
          {unavailable ? (
            <p>
              {t(
                'Cost is unavailable because consume logging is disabled or log statistics could not be read.'
              )}
            </p>
          ) : (
            <p>
              {t(
                "Cumulative upstream cost from retained consume logs, using each request's recorded base quota and channel cost ratio."
              )}
            </p>
          )}
          {props.incomplete && !unavailable && (
            <p>
              {t(
                'Historical records without a verified request-time cost ratio or base quota are excluded.'
              )}
            </p>
          )}
        </div>
      </TooltipContent>
    </Tooltip>
  )
}
