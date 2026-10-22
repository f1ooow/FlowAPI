/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/

import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import { describe, expect, test } from 'vitest'

import { Button } from '@/components/ui/button'

import { ChannelsPrimaryButtons } from '../channels-primary-buttons'
import { ChannelsProvider } from '../channels-provider'

describe('ChannelsPrimaryButtons layout', () => {
  test('renders batch mode control at the same height as the history button', () => {
    const queryClient = new QueryClient()

    render(
      <QueryClientProvider client={queryClient}>
        <ChannelsProvider>
          <ChannelsPrimaryButtons
            historyButton={
              <Button variant='outline' size='sm'>
                History and totals
              </Button>
            }
          />
        </ChannelsProvider>
      </QueryClientProvider>
    )

    const history = screen.getByRole('button', { name: 'History and totals' })
    const batchMode = screen.getByRole('group', { name: 'Batch Operations' })
    expect(history).toHaveClass('h-7')
    expect(batchMode).toHaveClass('h-7')
    expect(history.parentElement).toBe(batchMode.parentElement)
    expect(history.parentElement).toHaveClass('auto-cols-fr')
    queryClient.clear()
  })
})
