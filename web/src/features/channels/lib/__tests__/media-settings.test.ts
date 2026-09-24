import { describe, expect, test } from 'vitest'

import { isValidTaskEndpointPath } from '../media-settings'

describe('task endpoint settings', () => {
  test('accepts a relative submit path without placeholders', () => {
    expect(isValidTaskEndpointPath('/v1/videos/generations', false)).toBe(true)
    expect(isValidTaskEndpointPath('/v1/videos/{task_id}', false)).toBe(false)
  })

  test('requires exactly one task id in fetch paths', () => {
    expect(isValidTaskEndpointPath('/api/v1/tasks/{task_id}', true)).toBe(true)
    expect(isValidTaskEndpointPath('/api/v1/tasks', true)).toBe(false)
    expect(isValidTaskEndpointPath('/api/v1/tasks/{task_id}/{task_id}', true)).toBe(false)
  })

  test('rejects external hosts and traversal segments', () => {
    expect(isValidTaskEndpointPath('https://provider.example/tasks', true)).toBe(false)
    expect(isValidTaskEndpointPath('/api/../tasks/{task_id}', true)).toBe(false)
  })
})
