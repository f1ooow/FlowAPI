import { describe, expect, it } from 'vitest'

import { resolveLegacyRoute } from '../legacy-route'

describe('monitoring navigation', () => {
  it('opens the integrated channel page from either historical monitoring URL', () => {
    expect(resolveLegacyRoute('/channel-monitoring')).toBe('/channels')
    expect(resolveLegacyRoute('/channel-monitoring/')).toBe('/channels')
    expect(resolveLegacyRoute('/group-monitoring')).toBeNull()
  })
})
