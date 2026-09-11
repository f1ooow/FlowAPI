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
export const MODEL_REDIRECT_MATCH_TYPES = [
  'exact',
  'prefix',
  'suffix',
  'contains',
  'regex',
] as const

const MAX_MODEL_REDIRECT_RULES = 100
const MAX_MODEL_REDIRECT_SOURCE_LENGTH = 512
const MAX_MODEL_REDIRECT_TARGET_LENGTH = 255

export type ModelRedirectMatchType = (typeof MODEL_REDIRECT_MATCH_TYPES)[number]

export type ModelRedirectRule = {
  match_type: ModelRedirectMatchType
  source: string
  target: string
}

export type ModelRedirectMatch = {
  index: number
  rule: ModelRedirectRule
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

function isMatchType(value: unknown): value is ModelRedirectMatchType {
  return MODEL_REDIRECT_MATCH_TYPES.includes(value as ModelRedirectMatchType)
}

export function normalizeModelRedirectRules(
  value: unknown
): ModelRedirectRule[] {
  if (isRecord(value)) {
    return Object.entries(value)
      .sort(([left], [right]) => {
        if (left < right) return -1
        if (left > right) return 1
        return 0
      })
      .map(([source, target]) => ({
        match_type: 'exact',
        source,
        target: typeof target === 'string' ? target : '',
      }))
  }

  if (!Array.isArray(value)) return []

  return value.map((item) => {
    if (!isRecord(item)) {
      return { match_type: 'exact', source: '', target: '' }
    }
    return {
      match_type: isMatchType(item.match_type) ? item.match_type : 'exact',
      source: typeof item.source === 'string' ? item.source : '',
      target: typeof item.target === 'string' ? item.target : '',
    }
  })
}

export function parseModelRedirectRules(value: string): ModelRedirectRule[] {
  if (!value.trim()) return []
  return normalizeModelRedirectRules(JSON.parse(value))
}

export function stringifyModelRedirectRules(
  rules: ModelRedirectRule[]
): string {
  if (rules.length === 0) return ''
  return JSON.stringify(rules, null, 2)
}

export function normalizeModelRedirectJson(
  value: string | null | undefined
): string {
  if (!value?.trim()) return ''
  try {
    return stringifyModelRedirectRules(parseModelRedirectRules(value))
  } catch {
    return value
  }
}

function ruleMatches(model: string, rule: ModelRedirectRule): boolean {
  switch (rule.match_type) {
    case 'exact':
      return model === rule.source
    case 'prefix':
      return model.startsWith(rule.source)
    case 'suffix':
      return model.endsWith(rule.source)
    case 'contains':
      return model.includes(rule.source)
    case 'regex':
      try {
        return new RegExp(rule.source).test(model)
      } catch {
        return false
      }
  }
}

export function findFirstModelRedirect(
  model: string,
  rules: ModelRedirectRule[]
): ModelRedirectMatch | null {
  for (let index = 0; index < rules.length; index += 1) {
    const rule = rules[index]
    if (rule.source && rule.target && ruleMatches(model, rule)) {
      return { index, rule }
    }
  }
  return null
}

export function validateModelRedirectRules(value: string): {
  valid: boolean
  error?: string
} {
  if (!value.trim()) return { valid: true }

  let parsed: unknown
  try {
    parsed = JSON.parse(value)
  } catch {
    return { valid: false, error: 'Model redirect rules must be valid JSON' }
  }

  if (!Array.isArray(parsed) && !isRecord(parsed)) {
    return {
      valid: false,
      error: 'Model redirect rules must be a JSON array',
    }
  }

  if (isRecord(parsed)) {
    if (Object.values(parsed).some((target) => typeof target !== 'string')) {
      return {
        valid: false,
        error: 'Legacy model mapping values must be strings',
      }
    }
    return { valid: true }
  }

  const seen = new Set<string>()
  if (parsed.length > MAX_MODEL_REDIRECT_RULES) {
    return { valid: false, error: 'Too many model redirect rules' }
  }
  for (const item of parsed) {
    if (!isRecord(item)) {
      return {
        valid: false,
        error: 'Each model redirect rule must be an object',
      }
    }
    if (!isMatchType(item.match_type)) {
      return { valid: false, error: 'Invalid model redirect match type' }
    }
    if (
      typeof item.source !== 'string' ||
      !item.source.trim() ||
      typeof item.target !== 'string' ||
      !item.target.trim()
    ) {
      return {
        valid: false,
        error: 'Model redirect source and target are required',
      }
    }
    if (
      item.source.length > MAX_MODEL_REDIRECT_SOURCE_LENGTH ||
      item.target.length > MAX_MODEL_REDIRECT_TARGET_LENGTH
    ) {
      return { valid: false, error: 'Model redirect rule is too long' }
    }
    if (item.match_type === 'regex') {
      try {
        new RegExp(item.source)
      } catch {
        return { valid: false, error: 'Model redirect regex is invalid' }
      }
    }
    const identity = `${item.match_type}\u0000${item.source}`
    if (seen.has(identity)) {
      return {
        valid: false,
        error: 'Duplicate model redirect rules are not allowed',
      }
    }
    seen.add(identity)
  }

  return { valid: true }
}
