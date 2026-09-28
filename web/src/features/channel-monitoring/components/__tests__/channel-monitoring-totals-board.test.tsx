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
import i18next from 'i18next'
import { afterEach, beforeEach, describe, expect, test } from 'vitest'

import { formatQuotaWithCurrency } from '@/lib/currency'

import type { ChannelMonitoringTotalsWindow } from '../../types'
import { ChannelMonitoringTotalsBoard } from '../channel-monitoring-totals-board'

const WINDOW: ChannelMonitoringTotalsWindow = {
  used_quota: 500_000,
  cost_quota: 200_000,
  profit: 300_000,
  cost_has_estimate: false,
}

describe('ChannelMonitoringTotalsBoard', () => {
  // The app names its Chinese locale 'zhCN', which is not a BCP 47 tag:
  // Intl.NumberFormat rejects it with a RangeError. The bundle needs a key so
  // i18next resolves to it, as it does in the real app.
  beforeEach(async () => {
    i18next.addResourceBundle('zhCN', 'translation', { Today: '今日' })
    await i18next.changeLanguage('zhCN')
  })

  afterEach(async () => {
    await i18next.changeLanguage('en')
    i18next.removeResourceBundle('zhCN', 'translation')
  })

  test('formats amounts under the app Chinese language code', () => {
    render(
      <ChannelMonitoringTotalsBoard
        windows={{ today: WINDOW, month: WINDOW, total: WINDOW }}
        selectedWindow='today'
        onSelectWindow={() => undefined}
      />
    )

    const used = formatQuotaWithCurrency(WINDOW.used_quota, {
      fixedFractionDigits: 2,
      locale: 'zh-CN',
    })
    expect(screen.getAllByText(used, { exact: false })).toHaveLength(3)
  })
})
