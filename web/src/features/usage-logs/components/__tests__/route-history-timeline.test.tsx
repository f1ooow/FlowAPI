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
import i18next from 'i18next'
import { beforeAll, describe, expect, test } from 'vitest'

import { RouteHistoryTimeline } from '../route-history-timeline'

describe('route history timeline', () => {
  test('shows losing and threshold events as racing states rather than successful delivered responses', () => {
    render(
      <RouteHistoryTimeline
        attempts={[
          {
            channel_id: 301,
            attempt: 1,
            outcome: 'hedge_threshold',
            reason: 'first_content_timeout',
          },
          { channel_id: 301, attempt: 1, outcome: 'hedge_loser' },
          { channel_id: 301, attempt: 1, outcome: 'hedge_draining' },
          {
            channel_id: 301,
            attempt: 1,
            outcome: 'hedge_cancelled',
            reason: 'loser_billing_policy',
          },
          {
            channel_id: 302,
            attempt: 2,
            outcome: 'hedge_winner',
            reason: 'first_content',
          },
        ]}
      />
    )
    expect(screen.getByText('Extra racing attempt')).toBeInTheDocument()
    expect(screen.getByText('Racing winner')).toBeInTheDocument()
    expect(
      screen.getByText('First-content threshold reached')
    ).toBeInTheDocument()
    expect(screen.getByText('Background metering')).toBeInTheDocument()
    expect(screen.getByText('Racing attempt cancelled')).toBeInTheDocument()
    expect(screen.getByText(/Loser billing policy/)).toBeInTheDocument()
    expect(screen.queryByText('Success')).not.toBeInTheDocument()
  })

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
