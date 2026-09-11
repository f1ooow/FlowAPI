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
import {
  parseModelRedirectRules,
  validateModelRedirectRules,
} from './model-redirect-rules'

// ============================================================================
// Model Mapping Validation Utilities
// ============================================================================

/**
 * Parse models string to array
 */
export function parseModelsString(modelsStr: string): string[] {
  return modelsStr
    ? modelsStr
        .split(',')
        .map((m) => m.trim())
        .filter(Boolean)
    : []
}

/**
 * Format models array to string
 */
export function formatModelsArray(models: string[]): string {
  return [...new Set(models)].join(',')
}

/**
 * Normalize model name
 */
export function normalizeModelName(model: string): string {
  return typeof model === 'string' ? model.trim() : ''
}

/**
 * Extract exact source models from model redirect rules.
 */
export function extractMappingSourceModels(modelMapping: string): string[] {
  if (typeof modelMapping !== 'string') return []
  const trimmed = modelMapping.trim()
  if (!trimmed) return []

  try {
    const keys = parseModelRedirectRules(trimmed)
      .filter((rule) => rule.match_type === 'exact')
      .map((rule) => rule.source.trim())
      .filter(Boolean)

    return [...new Set(keys)]
  } catch {
    return []
  }
}

/**
 * Extract redirect models from model_mapping JSON
 */
export function extractRedirectModels(modelMapping: string): string[] {
  const mapping = modelMapping
  if (typeof mapping !== 'string') return []
  const trimmed = mapping.trim()
  if (!trimmed) return []

  try {
    const values = parseModelRedirectRules(trimmed)
      .map((rule) => rule.target.trim())
      .filter(Boolean)

    return [...new Set(values)]
  } catch {
    return []
  }
}

/**
 * Validate model mapping JSON format
 */
export function validateModelMappingJson(modelMapping: string): {
  valid: boolean
  error?: string
} {
  if (!modelMapping || modelMapping.trim() === '') {
    return { valid: true }
  }

  return validateModelRedirectRules(modelMapping)
}

/**
 * Get redirect models that are also in the models list
 * (These should be removed from models list to keep /v1/models clean)
 */
export function findExposedTargetModels(
  modelMapping: string,
  currentModels: string[]
): string[] {
  const redirectModels = extractRedirectModels(modelMapping)
  if (redirectModels.length === 0) return []

  const normalizedModels = currentModels.map((m) => normalizeModelName(m))
  const modelSet = new Set(normalizedModels)

  return redirectModels.filter((model) =>
    modelSet.has(normalizeModelName(model))
  )
}

/**
 * Categorize models into different sets for UI display
 */
export function categorizeModelsWithRedirect(
  currentModels: string[],
  redirectModels: string[]
): {
  normalizedCurrentModels: Set<string>
  normalizedRedirectModels: Set<string>
  classificationSet: Set<string>
  redirectOnlySet: Set<string>
} {
  const normalizedCurrentModels = new Set(
    currentModels.map((m) => normalizeModelName(m)).filter(Boolean)
  )

  const normalizedRedirectModels = new Set(
    redirectModels.map((m) => normalizeModelName(m)).filter(Boolean)
  )

  const classificationSet = new Set([
    ...normalizedCurrentModels,
    ...normalizedRedirectModels,
  ])

  const redirectOnlySet = new Set(
    [...normalizedRedirectModels].filter((m) => !normalizedCurrentModels.has(m))
  )

  return {
    normalizedCurrentModels,
    normalizedRedirectModels,
    classificationSet,
    redirectOnlySet,
  }
}
