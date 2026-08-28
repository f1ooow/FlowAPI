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
import { describe, expect, it } from 'vitest'

import { mapStatusDataToConfig } from '@/hooks/use-system-config'
import { DEFAULT_LOGO, DEFAULT_SYSTEM_NAME } from '@/lib/constants'

describe('mapStatusDataToConfig branding', () => {
  it('migrates the legacy default identity to Flow API', () => {
    const config = mapStatusDataToConfig({
      system_name: 'New API',
      logo: '/logo.png',
    })

    expect(config.systemName).toBe(DEFAULT_SYSTEM_NAME)
    expect(config.logo).toBe(DEFAULT_LOGO)
  })

  it('preserves an administrator-defined identity', () => {
    const config = mapStatusDataToConfig({
      system_name: 'Internal Gateway',
      logo: 'https://assets.example.com/gateway.svg',
    })

    expect(config.systemName).toBe('Internal Gateway')
    expect(config.logo).toBe('https://assets.example.com/gateway.svg')
  })

  it('migrates superseded Flow API logo assets to the current default', () => {
    const config = mapStatusDataToConfig({
      system_name: 'Flow API',
      logo: '/flow-api-logo.svg',
    })

    expect(config.logo).toBe(DEFAULT_LOGO)
  })
})
