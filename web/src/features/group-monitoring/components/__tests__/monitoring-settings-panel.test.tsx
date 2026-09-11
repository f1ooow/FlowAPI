/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the License,
or (at your option) any later version.
*/

import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, test, vi } from 'vitest'

import type { GroupMonitoringSetting } from '../../types'
import { MonitoringSettingsPanel } from '../monitoring-settings-panel'

const AVAILABLE_MODELS = {
  default: ['gpt-4o-mini'],
  vip: ['claude-sonnet-4-6', 'gpt-image-2'],
}

const SETTING: GroupMonitoringSetting = {
  enabled: true,
  bucket_minutes: 5,
  groups: [
    { group: 'default', description: 'core traffic', models: ['gpt-4o-mini'] },
  ],
}

function renderPanel(storageBucketMinutes = 5) {
  const onSave = vi.fn()
  render(
    <MonitoringSettingsPanel
      availableGroups={['default', 'vip']}
      availableModelsByGroup={AVAILABLE_MODELS}
      storageBucketMinutes={storageBucketMinutes}
      setting={SETTING}
      saving={false}
      onSave={onSave}
    />
  )
  return onSave
}

describe('MonitoringSettingsPanel', () => {
  test('shows per-group configuration only for the checked groups', () => {
    renderPanel()

    expect(screen.getByRole('checkbox', { name: 'default' })).toBeChecked()
    expect(screen.getByRole('checkbox', { name: 'vip' })).not.toBeChecked()
    expect(screen.getAllByLabelText('Description')).toHaveLength(1)
    expect(screen.getByLabelText('Description')).toHaveValue('core traffic')
    expect(screen.getAllByLabelText('Monitored models')).toHaveLength(1)
  })

  test('checking a group reveals its own model selector seeded from that group', async () => {
    const user = userEvent.setup()
    renderPanel()

    await user.click(screen.getByRole('checkbox', { name: 'vip' }))

    expect(screen.getAllByLabelText('Monitored models')).toHaveLength(2)
    expect(screen.getAllByLabelText('Description')).toHaveLength(2)
    expect(screen.getByText('claude-sonnet-4-6')).toBeInTheDocument()
    expect(screen.queryByText('gpt-image-2')).toBeNull()
  })

  test('unchecking a group drops it from the saved settings', async () => {
    const user = userEvent.setup()
    const onSave = renderPanel()

    await user.click(screen.getByRole('checkbox', { name: 'default' }))
    expect(screen.queryByLabelText('Description')).toBeNull()

    await user.click(screen.getByRole('button', { name: 'Save settings' }))

    await waitFor(() =>
      expect(onSave).toHaveBeenCalledWith({
        enabled: true,
        bucket_minutes: 5,
        groups: [],
      })
    )
  })

  test('rejects saving a monitored group with no models selected', async () => {
    const user = userEvent.setup()
    const onSave = renderPanel()

    await user.click(screen.getByRole('checkbox', { name: 'vip' }))
    await user.click(screen.getByRole('button', { name: 'Save settings' }))
    await waitFor(() => expect(onSave).toHaveBeenCalled())

    onSave.mockClear()
    // Removing the seeded model leaves the group empty, which the API rejects.
    await user.clear(screen.getAllByLabelText('Monitored models')[1])
    await user.keyboard('{Backspace}')
    await user.click(screen.getByRole('button', { name: 'Save settings' }))

    await waitFor(() =>
      expect(screen.getAllByRole('alert').length).toBeGreaterThan(0)
    )
    expect(onSave).not.toHaveBeenCalled()
  })

  test('stores a newly checked group in the order it is listed, not appended', async () => {
    const user = userEvent.setup()
    const onSave = vi.fn()
    render(
      <MonitoringSettingsPanel
        availableGroups={['default', 'vip']}
        availableModelsByGroup={AVAILABLE_MODELS}
        storageBucketMinutes={5}
        setting={{
          enabled: true,
          bucket_minutes: 5,
          groups: [{ group: 'vip', description: '', models: ['gpt-image-2'] }],
        }}
        saving={false}
        onSave={onSave}
      />
    )

    await user.click(screen.getByRole('checkbox', { name: 'default' }))
    await user.click(screen.getByRole('button', { name: 'Save settings' }))

    await waitFor(() => expect(onSave).toHaveBeenCalled())
    const saved = onSave.mock.calls[0][0] as GroupMonitoringSetting
    expect(saved.groups.map((group) => group.group)).toEqual(['default', 'vip'])
  })

  test('keeps a group that no longer exists visible so it can be unchecked', async () => {
    const user = userEvent.setup()
    const onSave = vi.fn()
    render(
      <MonitoringSettingsPanel
        availableGroups={['default']}
        availableModelsByGroup={{ default: ['gpt-4o-mini'] }}
        storageBucketMinutes={5}
        setting={{
          enabled: true,
          bucket_minutes: 5,
          groups: [{ group: 'retired', description: '', models: ['old'] }],
        }}
        saving={false}
        onSave={onSave}
      />
    )

    const retired = screen.getByRole('checkbox', { name: 'retired' })
    expect(retired).toBeChecked()

    await user.click(retired)
    await user.click(screen.getByRole('button', { name: 'Save settings' }))

    await waitFor(() =>
      expect(onSave).toHaveBeenCalledWith(
        expect.objectContaining({ groups: [] })
      )
    )
  })

  test('disables timeline buckets narrower than the storage bucket', () => {
    renderPanel(60)

    expect(screen.getByRole('option', { name: '5 minutes' })).toBeDisabled()
    expect(screen.getByRole('option', { name: '30 minutes' })).toBeDisabled()
    expect(screen.getByRole('option', { name: '60 minutes' })).toBeEnabled()
  })
})
