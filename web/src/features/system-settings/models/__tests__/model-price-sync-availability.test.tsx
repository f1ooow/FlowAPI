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
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import type { PropsWithChildren } from 'react'
import { describe, expect, test, vi } from 'vitest'

import { RatioSettingsCard } from '../ratio-settings-card'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string) => key,
  }),
}))

vi.mock('../../components/settings-page-context', () => ({
  SettingsPageTitleStatusPortal: (props: PropsWithChildren) => props.children,
  useSuppressSettingsSectionHeader: () => false,
}))

vi.mock('../model-ratio-form', () => ({
  ModelRatioForm: () => null,
}))

vi.mock('../group-ratio-form', () => ({
  GroupRatioForm: () => null,
}))

vi.mock('../tool-price-settings', () => ({
  ToolPriceSettings: () => null,
}))

vi.mock('../upstream-ratio-sync', () => ({
  UpstreamRatioSync: () => null,
}))

const modelDefaults = {
  ModelPrice: '{}',
  ModelRatio: '{}',
  CacheRatio: '{}',
  CreateCacheRatio: '{}',
  CompletionRatio: '{}',
  ImageRatio: '{}',
  AudioRatio: '{}',
  AudioCompletionRatio: '{}',
  ExposeRatioEnabled: false,
  BillingMode: '{}',
  BillingExpr: '{}',
}

const groupDefaults = {
  GroupRatio: '{}',
  TopupGroupRatio: '{}',
  UserUsableGroups: '{}',
  UserGroupRatio: '{}',
  IncludeChannelRatio: '{}',
  MigrationConflicts: '[]',
  AutoGroups: '[]',
  MaxTokenAutoGroups: 5,
  DefaultUseAutoGroup: false,
  GroupSpecialUsableGroup: '{}',
}

describe('model pricing navigation', () => {
  test('shows global upstream price sync as a model-pricing tab', () => {
    const queryClient = new QueryClient({
      defaultOptions: {
        queries: { retry: false },
        mutations: { retry: false },
      },
    })

    render(
      <QueryClientProvider client={queryClient}>
        <RatioSettingsCard
          modelDefaults={modelDefaults}
          groupDefaults={groupDefaults}
          toolPricesDefault='{}'
          visibleTabs={[
            'models',
            'unset-models',
            'tool-prices',
            'upstream-sync',
          ]}
        />
      </QueryClientProvider>
    )

    expect(
      screen.getByRole('tab', { name: 'Upstream price sync' })
    ).toBeVisible()

    queryClient.clear()
  })
})
