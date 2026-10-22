import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, test, vi } from 'vitest'

import type { GroupMonitoringSetting } from '../../types'
import { MonitoringSettingsPanel } from '../monitoring-settings-panel'

const AVAILABLE_MODELS = {
  default: ['claude-sonnet', 'gpt-image'],
  vip: ['claude-sonnet', 'claude-opus'],
}

const SETTING: GroupMonitoringSetting = {
  enabled: true,
  bucket_minutes: 5,
  groups: [],
}

function renderPanel(setting = SETTING, storageBucketMinutes = 5) {
  const onSave = vi.fn()
  render(
    <MonitoringSettingsPanel
      availableGroups={['default', 'vip']}
      availableModelsByGroup={AVAILABLE_MODELS}
      storageBucketMinutes={storageBucketMinutes}
      setting={setting}
      saving={false}
      onSave={onSave}
    />
  )
  return onSave
}

describe('MonitoringSettingsPanel', () => {
  test('creates a named monitor with multiple sources and only their common model', async () => {
    const user = userEvent.setup()
    const onSave = renderPanel()

    await user.click(screen.getByRole('button', { name: 'Add monitor' }))
    await user.type(screen.getByLabelText('Monitor name'), 'Claude Max')
    await user.click(screen.getByLabelText('Source groups'))
    await user.click(screen.getByRole('option', { name: 'default' }))
    await user.click(screen.getByRole('option', { name: 'vip' }))
    await user.keyboard('{Escape}')
    await user.click(screen.getByLabelText('Monitored models'))
    expect(screen.queryByRole('option', { name: 'gpt-image' })).toBeNull()
    expect(screen.queryByRole('option', { name: 'claude-opus' })).toBeNull()
    await user.click(screen.getByRole('option', { name: 'claude-sonnet' }))
    await user.keyboard('{Escape}')
    await user.click(screen.getByRole('button', { name: 'Save settings' }))

    await waitFor(() =>
      expect(onSave).toHaveBeenCalledWith({
        enabled: true,
        bucket_minutes: 5,
        groups: [
          {
            id: expect.any(String),
            name: 'Claude Max',
            groups: ['default', 'vip'],
            models: ['claude-sonnet'],
            description: '',
            visible_to_groups: null,
          },
        ],
      })
    )
  })
})
