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
  ArrowDown,
  ArrowRight,
  ArrowUp,
  Code,
  Plus,
  Search,
  Table,
  Trash2,
} from 'lucide-react'
import { useCallback, useEffect, useId, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { JsonCodeEditor } from '@/components/json-code-editor'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'

import {
  findFirstModelRedirect,
  MODEL_REDIRECT_MATCH_TYPES,
  parseModelRedirectRules,
  stringifyModelRedirectRules,
  type ModelRedirectMatchType,
  type ModelRedirectRule,
  validateModelRedirectRules,
} from '../lib/model-redirect-rules'

type ModelMappingEditorProps = {
  value: string
  onChange: (value: string) => void
  disabled?: boolean
  sourceModelOptions?: string[]
  targetModelOptions?: string[]
}

type RuleRow = ModelRedirectRule & { id: string }

const MATCH_TYPE_LABELS: Record<ModelRedirectMatchType, string> = {
  exact: 'Exact match',
  prefix: 'Prefix match',
  suffix: 'Suffix match',
  contains: 'Contains',
  regex: 'Regular expression',
}

export function ModelMappingEditor(props: ModelMappingEditorProps) {
  const { t } = useTranslation()
  const sourceListId = useId()
  const targetListId = useId()
  const [mode, setMode] = useState<'visual' | 'json'>('visual')
  const [rows, setRows] = useState<RuleRow[]>([])
  const [jsonValue, setJsonValue] = useState(props.value)
  const [jsonError, setJsonError] = useState<string | null>(null)
  const [testModel, setTestModel] = useState('')
  const [testRequested, setTestRequested] = useState(false)
  const nextRowIdRef = useRef(0)
  const lastAppliedValueRef = useRef<string | undefined>(undefined)

  const createRowId = useCallback(() => {
    nextRowIdRef.current += 1
    return `redirect-rule-${nextRowIdRef.current}`
  }, [])

  const rules = useMemo<ModelRedirectRule[]>(
    () =>
      rows.map((row) => ({
        match_type: row.match_type,
        source: row.source,
        target: row.target,
      })),
    [rows]
  )
  const testMatch = useMemo(
    () => findFirstModelRedirect(testModel, rules),
    [rules, testModel]
  )

  const applyParsedRules = useCallback(
    (value: string): boolean => {
      const validation = validateModelRedirectRules(value)
      if (!validation.valid) {
        setJsonError(t(validation.error || 'Invalid model redirect rules'))
        return false
      }

      try {
        const parsedRules = parseModelRedirectRules(value)
        setRows((previousRows) =>
          parsedRules.map((rule, index) => ({
            id: previousRows[index]?.id || createRowId(),
            ...rule,
          }))
        )
        setJsonError(null)
        return true
      } catch {
        setJsonError(t('Model redirect rules must be valid JSON'))
        return false
      }
    },
    [createRowId, t]
  )

  useEffect(() => {
    if (lastAppliedValueRef.current === props.value) return
    lastAppliedValueRef.current = props.value
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setJsonValue(props.value)
    applyParsedRules(props.value)
  }, [applyParsedRules, props.value])

  const syncRows = (updatedRows: RuleRow[]): void => {
    const updatedRules = updatedRows.map((row) => ({
      match_type: row.match_type,
      source: row.source,
      target: row.target,
    }))
    const json = stringifyModelRedirectRules(updatedRules)
    const validation = validateModelRedirectRules(json)
    setRows(updatedRows)
    setJsonValue(json)
    setJsonError(
      validation.valid ||
        updatedRows.some((row) => !row.source.trim() || !row.target.trim())
        ? null
        : t(validation.error || 'Invalid model redirect rules')
    )
    props.onChange(json)
  }

  const handleAddRule = (): void => {
    syncRows([
      ...rows,
      { id: createRowId(), match_type: 'exact', source: '', target: '' },
    ])
  }

  const updateRule = (id: string, patch: Partial<ModelRedirectRule>): void => {
    syncRows(rows.map((row) => (row.id === id ? { ...row, ...patch } : row)))
  }

  const moveRule = (index: number, direction: -1 | 1): void => {
    const targetIndex = index + direction
    if (targetIndex < 0 || targetIndex >= rows.length) return
    const updatedRows = [...rows]
    const [moved] = updatedRows.splice(index, 1)
    updatedRows.splice(targetIndex, 0, moved)
    syncRows(updatedRows)
  }

  const handleJsonChange = (value: string): void => {
    setJsonValue(value)
    props.onChange(value)
    applyParsedRules(value)
  }

  const handleFillTemplate = (): void => {
    const value = stringifyModelRedirectRules([
      {
        match_type: 'contains',
        source: 'opus',
        target: 'claude-opus-4-6',
      },
      {
        match_type: 'contains',
        source: 'sonnet',
        target: 'claude-sonnet-4-6',
      },
    ])
    setJsonValue(value)
    props.onChange(value)
    applyParsedRules(value)
  }

  const handleModeChange = (nextMode: string): void => {
    if (nextMode !== 'visual' && nextMode !== 'json') return
    if (nextMode === 'json') {
      const normalized = stringifyModelRedirectRules(rules)
      setJsonValue(normalized)
      props.onChange(normalized)
    } else {
      applyParsedRules(jsonValue)
    }
    setMode(nextMode)
  }

  return (
    <div className='space-y-4'>
      <Tabs value={mode} onValueChange={handleModeChange} className='space-y-3'>
        <div className='flex items-center justify-between gap-3'>
          <TabsList>
            <TabsTrigger value='visual'>
              <Table className='h-4 w-4' aria-hidden='true' />
              {t('Visual')}
            </TabsTrigger>
            <TabsTrigger value='json'>
              <Code className='h-4 w-4' aria-hidden='true' />
              {t('JSON')}
            </TabsTrigger>
          </TabsList>
          <Button
            type='button'
            variant='link'
            size='sm'
            className='h-auto p-0'
            onClick={handleFillTemplate}
            disabled={props.disabled}
          >
            {t('Fill Template')}
          </Button>
        </div>

        {jsonError && (
          <Alert variant='destructive'>
            <AlertDescription>{jsonError}</AlertDescription>
          </Alert>
        )}

        <TabsContent value='visual' className='space-y-3'>
          {rows.length > 0 ? (
            <div className='space-y-2'>
              {rows.map((row, index) => (
                <div
                  key={row.id}
                  className='border-border/60 grid gap-2 rounded-md border p-3 lg:grid-cols-[9rem_minmax(10rem,1fr)_auto_minmax(10rem,1fr)_auto] lg:items-center'
                >
                  <Select
                    value={row.match_type}
                    onValueChange={(value) =>
                      updateRule(row.id, {
                        match_type: value as ModelRedirectMatchType,
                      })
                    }
                    disabled={props.disabled}
                  >
                    <SelectTrigger
                      className='h-10 w-full'
                      aria-label={t('Match type')}
                    >
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {MODEL_REDIRECT_MATCH_TYPES.map((matchType) => (
                        <SelectItem key={matchType} value={matchType}>
                          {t(MATCH_TYPE_LABELS[matchType])}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                  <Input
                    value={row.source}
                    onChange={(event) =>
                      updateRule(row.id, { source: event.target.value })
                    }
                    placeholder={t('User-requested model or pattern')}
                    aria-label={t('User-requested model or pattern')}
                    disabled={props.disabled}
                    list={sourceListId}
                  />
                  <ArrowRight
                    className='text-muted-foreground hidden h-4 w-4 lg:block'
                    aria-hidden='true'
                  />
                  <Input
                    value={row.target}
                    onChange={(event) =>
                      updateRule(row.id, { target: event.target.value })
                    }
                    placeholder={t('Actual upstream model')}
                    aria-label={t('Actual upstream model')}
                    disabled={props.disabled}
                    list={targetListId}
                  />
                  <div className='flex justify-end gap-1'>
                    <Button
                      type='button'
                      variant='ghost'
                      size='icon-sm'
                      onClick={() => moveRule(index, -1)}
                      disabled={props.disabled || index === 0}
                      aria-label={t('Move rule up')}
                    >
                      <ArrowUp aria-hidden='true' />
                    </Button>
                    <Button
                      type='button'
                      variant='ghost'
                      size='icon-sm'
                      onClick={() => moveRule(index, 1)}
                      disabled={props.disabled || index === rows.length - 1}
                      aria-label={t('Move rule down')}
                    >
                      <ArrowDown aria-hidden='true' />
                    </Button>
                    <Button
                      type='button'
                      variant='ghost'
                      size='icon-sm'
                      onClick={() =>
                        syncRows(rows.filter((item) => item.id !== row.id))
                      }
                      disabled={props.disabled}
                      aria-label={t('Delete rule')}
                    >
                      <Trash2 aria-hidden='true' />
                    </Button>
                  </div>
                </div>
              ))}
            </div>
          ) : (
            <div className='text-muted-foreground flex h-24 items-center justify-center rounded-md border border-dashed px-4 text-center text-sm'>
              {t('No model redirect rules configured.')}
            </div>
          )}
          <Button
            type='button'
            variant='outline'
            size='sm'
            onClick={handleAddRule}
            disabled={props.disabled}
            className='w-full'
          >
            <Plus aria-hidden='true' />
            {t('Add redirect rule')}
          </Button>
        </TabsContent>

        <TabsContent value='json'>
          <JsonCodeEditor
            value={jsonValue}
            onChange={handleJsonChange}
            placeholder='[{"match_type":"contains","source":"opus","target":"claude-opus-4-6"}]'
            disabled={props.disabled}
            className={jsonError ? 'border-destructive' : undefined}
            aria-invalid={Boolean(jsonError)}
            ariaLabel={t('Model redirect rules')}
          />
        </TabsContent>
      </Tabs>

      <div className='border-border/60 space-y-2 border-t pt-4'>
        <div className='flex flex-col gap-2 sm:flex-row'>
          <Input
            value={testModel}
            onChange={(event) => {
              setTestModel(event.target.value)
              setTestRequested(false)
            }}
            placeholder={t('Enter a model name to test')}
            aria-label={t('Model name to test')}
            disabled={props.disabled}
          />
          <Button
            type='button'
            variant='outline'
            onClick={() => setTestRequested(true)}
            disabled={props.disabled || !testModel.trim()}
          >
            <Search aria-hidden='true' />
            {t('Test rules')}
          </Button>
        </div>
        {testRequested && (
          <Alert>
            <AlertDescription>
              {testMatch ? (
                <span>
                  {t('Rule {{number}} matched', {
                    number: testMatch.index + 1,
                  })}
                  : <strong>{testMatch.rule.target}</strong>
                </span>
              ) : (
                t('No rule matched. The original model will be forwarded.')
              )}
            </AlertDescription>
          </Alert>
        )}
      </div>

      {props.sourceModelOptions && props.sourceModelOptions.length > 0 && (
        <datalist id={sourceListId}>
          {props.sourceModelOptions.map((model) => (
            <option key={model} value={model} />
          ))}
        </datalist>
      )}
      {props.targetModelOptions && props.targetModelOptions.length > 0 && (
        <datalist id={targetListId}>
          {props.targetModelOptions.map((model) => (
            <option key={model} value={model} />
          ))}
        </datalist>
      )}
    </div>
  )
}
