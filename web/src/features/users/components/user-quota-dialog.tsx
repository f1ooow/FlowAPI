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
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { getCurrencyDisplay, getCurrencyLabel } from '@/lib/currency'
import { formatQuota, parseQuotaFromDollars } from '@/lib/format'
import { cn } from '@/lib/utils'

import { adjustUserQuota, setUserUnlimitedQuota } from '../api'
import type { QuotaAdjustMode } from '../types'

interface UserQuotaDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  userId: number
  currentQuota: number
  currentUnlimited: boolean
  onSuccess: (unlimited: boolean) => void
}

export function UserQuotaDialog(props: UserQuotaDialogProps) {
  const { t } = useTranslation()
  const [mode, setMode] = useState<QuotaAdjustMode>('add')
  const [amount, setAmount] = useState('')
  const [loading, setLoading] = useState(false)
  const [unlimited, setUnlimited] = useState(props.currentUnlimited)

  useEffect(() => {
    if (props.open) {
      setUnlimited(props.currentUnlimited)
    }
  }, [props.currentUnlimited, props.open])

  const { meta: currencyMeta } = getCurrencyDisplay()
  const currencyLabel = getCurrencyLabel()
  const tokensOnly = currencyMeta.kind === 'tokens'

  const amountValue = Number.parseFloat(amount) || 0
  const quotaValue = parseQuotaFromDollars(Math.abs(amountValue))

  const getPreviewText = () => {
    const current = props.currentQuota
    const val = quotaValue
    switch (mode) {
      case 'add':
        return `${t('Current quota')}: ${formatQuota(current)}  +${formatQuota(val)} = ${formatQuota(current + val)}`
      case 'subtract':
        return `${t('Current quota')}: ${formatQuota(current)}  -${formatQuota(val)} = ${formatQuota(current - val)}`
      case 'override': {
        const overrideQuota = parseQuotaFromDollars(amountValue)
        return `${t('Current quota')}: ${formatQuota(current)} → ${formatQuota(overrideQuota)}`
      }
      default:
        return ''
    }
  }

  const handleConfirm = async () => {
    const unlimitedChanged = unlimited !== props.currentUnlimited
    const shouldAdjustQuota = !unlimited && amount !== ''
    if (!unlimitedChanged && !shouldAdjustQuota) return
    if (shouldAdjustQuota && quotaValue <= 0 && mode !== 'override') return

    setLoading(true)
    try {
      if (unlimitedChanged) {
        const result = await setUserUnlimitedQuota({
          id: props.userId,
          action: 'set_unlimited_quota',
          enabled: unlimited,
        })
        if (!result.success) {
          toast.error(result.message || t('Failed to update unlimited quota'))
          return
        }
      }

      if (shouldAdjustQuota) {
        const value =
          mode === 'override' ? parseQuotaFromDollars(amountValue) : quotaValue
        const result = await adjustUserQuota({
          id: props.userId,
          action: 'add_quota',
          mode,
          value: mode === 'override' ? value : Math.abs(value),
        })
        if (!result.success) {
          toast.error(result.message || t('Failed to adjust quota'))
          return
        }
      }

      toast.success(t('Quota settings updated'))
      setAmount('')
      setMode('add')
      props.onOpenChange(false)
      props.onSuccess(unlimited)
    } catch (e: unknown) {
      toast.error(e instanceof Error ? e.message : t('Failed to adjust quota'))
    } finally {
      setLoading(false)
    }
  }

  const handleCancel = () => {
    setAmount('')
    setMode('add')
    props.onOpenChange(false)
  }

  const placeholder = tokensOnly
    ? t('Enter amount in tokens')
    : t('Enter amount in {{currency}}', { currency: currencyLabel })

  return (
    <Dialog
      open={props.open}
      onOpenChange={props.onOpenChange}
      title={t('Adjust Quota')}
      description={t('Select an operation mode and enter the amount')}
      contentHeight='auto'
      bodyClassName='space-y-4'
      footer={
        <>
          <Button variant='outline' onClick={handleCancel}>
            {t('Cancel')}
          </Button>
          <Button onClick={handleConfirm} disabled={loading}>
            {loading ? t('Processing...') : t('Confirm')}
          </Button>
        </>
      }
    >
      <div className='space-y-4'>
        <div className='flex items-center justify-between gap-4 rounded-md border p-3'>
          <div className='min-w-0 space-y-1'>
            <Label htmlFor='user-unlimited-quota'>{t('Unlimited quota')}</Label>
            <p className='text-muted-foreground text-xs'>
              {t('Usage is tracked without deducting the user balance')}
            </p>
          </div>
          <Switch
            id='user-unlimited-quota'
            checked={unlimited}
            onCheckedChange={setUnlimited}
            disabled={loading}
          />
        </div>

        {!unlimited && (
          <>
            <div className='text-muted-foreground text-sm'>
              {getPreviewText()}
            </div>

            <div className='space-y-2'>
              <Label>{t('Mode')}</Label>
              <div className='flex gap-1'>
                {(['add', 'subtract', 'override'] as const).map((m) => {
                  let label = t('Override')
                  if (m === 'add') {
                    label = t('Add')
                  } else if (m === 'subtract') {
                    label = t('Subtract')
                  }

                  return (
                    <Button
                      key={m}
                      type='button'
                      variant='outline'
                      size='sm'
                      className={cn(
                        mode === m &&
                          'bg-primary text-primary-foreground hover:bg-primary/90 hover:text-primary-foreground'
                      )}
                      onClick={() => {
                        setMode(m)
                        setAmount('')
                      }}
                    >
                      {label}
                    </Button>
                  )
                })}
              </div>
            </div>

            <div className='space-y-2'>
              <Label>
                {t('Amount')} ({currencyLabel})
              </Label>
              <Input
                type='number'
                step={tokensOnly ? 1 : 0.000001}
                min={mode === 'override' ? undefined : 0}
                placeholder={placeholder}
                value={amount}
                onChange={(e) => setAmount(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === 'Enter') handleConfirm()
                }}
              />
            </div>
          </>
        )}
      </div>
    </Dialog>
  )
}
