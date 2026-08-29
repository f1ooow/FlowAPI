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
import type { ReactNode } from 'react'
import { describe, expect, it, vi } from 'vitest'

import { SystemBrand } from '../system-brand'

vi.mock('@tanstack/react-router', () => ({
  Link: (props: {
    to: string
    children?: ReactNode
    'aria-label'?: string
    className?: string
  }) => (
    <a
      href={props.to}
      aria-label={props['aria-label']}
      className={props.className}
    >
      {props.children}
    </a>
  ),
}))

vi.mock('@/hooks/use-status', () => ({
  useStatus: () => ({
    status: {
      system_name: 'New API',
      version: 'v1.0.0',
    },
  }),
}))

vi.mock('@/hooks/use-system-config', () => ({
  useSystemConfig: () => ({ logo: '/logo.png' }),
}))

describe('SystemBrand', () => {
  it('restores the inline New API brand and home link in the app header', () => {
    render(<SystemBrand variant='inline' />)

    const homeLink = screen.getByRole('link', { name: 'Go to home' })
    expect(homeLink).toHaveAttribute('href', '/')
    expect(homeLink).toHaveTextContent('New API')
    expect(screen.getByRole('img', { name: 'Logo' })).toHaveAttribute(
      'src',
      '/logo.png'
    )
  })
})
