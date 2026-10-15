/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.
*/
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import i18next from 'i18next'
import { beforeAll, describe, expect, test } from 'vitest'

import { TooltipProvider } from '@/components/ui/tooltip'

import { RouteHistoryTimeline } from '../route-history-timeline'

describe('route history timeline', () => {
  beforeAll(() => {
    i18next.addResourceBundle('en', 'translation', {
      Success: 'Success',
      Retry: 'Retry',
      Exhausted: 'Exhausted',
      Stopped: 'Stopped',
      'Attempt {{number}}': 'Attempt {{number}}',
      Priority: 'Priority',
      Weight: 'Weight',
      Reason: 'Reason',
      'Invalid upstream response': 'Invalid upstream response',
      'Attempts exhausted': 'Attempts exhausted',
    })
  })

  test('shows the concrete provider for every attempt, including repeated attempts', () => {
    render(
      <RouteHistoryTimeline
        attempts={[
          {
            channel_id: 301,
            channel_name: 'Fastapi Codex',
            attempt: 1,
            outcome: 'retrying_channel',
            reason: 'invalid_precommit_upstream_response',
            status_code: 502,
            priority: 11,
            weight: 1,
          },
          {
            channel_id: 301,
            channel_name: 'Fastapi Codex',
            attempt: 2,
            outcome: 'channel_exhausted',
            reason: 'attempts_exhausted',
            status_code: 502,
            priority: 11,
            weight: 1,
          },
          {
            channel_id: 302,
            channel_name: 'SC CODE PLUS 稳定',
            attempt: 1,
            outcome: 'succeeded',
            status_code: 200,
          },
        ]}
      />
    )

    expect(screen.getAllByText('Fastapi Codex')).toHaveLength(2)
    expect(screen.getByText('SC CODE PLUS 稳定')).toBeInTheDocument()
    expect(screen.getAllByText('HTTP 502')).toHaveLength(2)
    expect(screen.getByText('HTTP 200')).toBeInTheDocument()
    expect(screen.getAllByText('Attempt 1')).toHaveLength(2)
    expect(screen.getByText('Attempt 2')).toBeInTheDocument()
    expect(
      screen.getByTitle('invalid_precommit_upstream_response')
    ).toBeInTheDocument()
    expect(screen.getByTitle('attempts_exhausted')).toBeInTheDocument()
  })

  test('does not mislabel a multi-channel historical chain with the final channel name', () => {
    render(
      <RouteHistoryTimeline
        fallbackChannelName='Final channel'
        attempts={[
          {
            channel_id: 301,
            attempt: 1,
            outcome: 'retrying_channel',
            status_code: 502,
          },
          {
            channel_id: 302,
            attempt: 1,
            outcome: 'succeeded',
            status_code: 200,
          },
        ]}
      />
    )

    expect(screen.queryByText('Final channel')).not.toBeInTheDocument()
    expect(screen.getAllByText('#301')).toHaveLength(2)
    expect(screen.getAllByText('#302')).toHaveLength(2)
  })
})

const timing = {
  started_at_ms: 1700000000150,
  ended_at_ms: 1700000012300,
  start_offset_ms: 150,
  end_offset_ms: 12300,
  duration_ms: 12150,
}

describe('route attempt timing', () => {
  test.each([
    ['retrying_channel', 'Failed at +12.30 s'],
    ['channel_exhausted', 'Failed at +12.30 s'],
    ['stopped', 'Stopped at +12.30 s'],
    ['succeeded', 'Success recorded at +12.30 s'],
  ])(
    'shows relative result time and attempt duration for %s',
    async (outcome, label) => {
      render(
        <TooltipProvider>
          <RouteHistoryTimeline
            attempts={[{ channel_id: 1, attempt: 1, outcome, timing }]}
          />
        </TooltipProvider>
      )
      const trigger = screen.getByRole('button', {
        name: `${label} · Attempt duration 12.15 s`,
      })
      await userEvent.setup().tab()
      expect(trigger).toHaveFocus()
      const tooltip = await screen.findByRole('tooltip')
      expect(tooltip).toHaveTextContent('Attempt started')
      expect(tooltip).toHaveTextContent('Result recorded')
      expect(tooltip).toHaveTextContent('2023')
      expect(tooltip).toHaveTextContent('.150')
      expect(tooltip).toHaveTextContent('.300')
      expect(tooltip).toHaveTextContent(
        Intl.DateTimeFormat().resolvedOptions().timeZone
      )
    }
  )

  test('renders genuine zero times and long durations without changing units', () => {
    render(
      <TooltipProvider>
        <RouteHistoryTimeline
          compact
          attempts={[
            {
              channel_id: 1,
              attempt: 1,
              outcome: 'retrying_channel',
              timing: {
                ...timing,
                start_offset_ms: 0,
                end_offset_ms: 0,
                duration_ms: 0,
              },
            },
            {
              channel_id: 2,
              attempt: 1,
              outcome: 'succeeded',
              timing: {
                ...timing,
                end_offset_ms: 3600123,
                duration_ms: 3600000,
              },
            },
          ]}
        />
      </TooltipProvider>
    )
    expect(
      screen.getByRole('button', {
        name: 'Failed at +0.00 s · Attempt duration 0.00 s',
      })
    ).toBeInTheDocument()
    expect(
      screen.getByRole('button', {
        name: 'Success recorded at +3600.12 s · Attempt duration 3600.00 s',
      })
    ).toBeInTheDocument()
  })

  test.each([
    undefined,
    { ...timing, duration_ms: -1 },
    { ...timing, end_offset_ms: Number.NaN },
    { ...timing, started_at_ms: Number.POSITIVE_INFINITY },
    { ...timing, ended_at_ms: 9e15 },
    { ...timing, end_offset_ms: 100 },
  ])(
    'shows missing-time fallback for historical or invalid timing %j',
    (invalidTiming) => {
      render(
        <RouteHistoryTimeline
          attempts={[
            {
              channel_id: 1,
              attempt: 1,
              outcome: 'succeeded',
              timing: invalidTiming,
            },
          ]}
        />
      )
      expect(screen.getByText('Timing not recorded')).toBeInTheDocument()
      expect(screen.queryByRole('button')).not.toBeInTheDocument()
    }
  )
})
