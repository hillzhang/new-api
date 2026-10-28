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
import { useQuery } from '@tanstack/react-query'
import {
  AlertTriangle,
  ChevronDown,
  ChevronUp,
  Download,
  Filter,
  Plus,
  RotateCcw,
  Sparkles,
  Trash2,
} from 'lucide-react'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { getPricing } from '@/features/pricing/api'

const sectionCardClassName =
  'relative shadow-sm ring-0 before:pointer-events-none before:absolute before:inset-0 before:rounded-xl before:border before:border-border/90'
const sectionHeaderClassName = 'border-b bg-muted/20'

type ModelRatioRule = {
  _id: string
  group: string
  model: string
  ratio: number | string
}

let _idCounter = 0
function uid() {
  return `gmr_${++_idCounter}`
}

function safeParseJson(str: string): Record<string, Record<string, number>> {
  if (!str || !str.trim()) return {}
  try {
    return JSON.parse(str) as Record<string, Record<string, number>>
  } catch {
    return {}
  }
}

function flattenRules(
  nested: Record<string, Record<string, number>>
): ModelRatioRule[] {
  const rules: ModelRatioRule[] = []
  for (const [group, models] of Object.entries(nested)) {
    if (typeof models !== 'object' || models === null) continue
    for (const [model, ratio] of Object.entries(models)) {
      rules.push({
        _id: uid(),
        group,
        model,
        ratio: typeof ratio === 'number' ? ratio : 1.0,
      })
    }
  }
  return rules
}

function serializeRules(rules: ModelRatioRule[]): string {
  const result: Record<string, Record<string, number>> = {}
  for (const { group, model, ratio } of rules) {
    const trimmedGroup = group.trim()
    const trimmedModel = model.trim()
    if (!trimmedGroup || !trimmedModel) continue

    const parsedRatio = typeof ratio === 'number' ? ratio : parseFloat(ratio)
    if (isNaN(parsedRatio) || parsedRatio < 0) continue

    if (!result[trimmedGroup]) result[trimmedGroup] = {}
    result[trimmedGroup][trimmedModel] = parsedRatio
  }
  return Object.keys(result).length === 0
    ? '{}'
    : JSON.stringify(result, null, 2)
}

type GroupSelectProps = {
  options: string[]
  value: string
  placeholder: string
  onValueChange: (value: string) => void
  className?: string
}

function GroupSelect(props: GroupSelectProps) {
  const knownOptions = useMemo(() => {
    if (props.value && !props.options.includes(props.value)) {
      return [props.value, ...props.options]
    }
    return props.options
  }, [props.options, props.value])

  return (
    <Select
      value={props.value === '' ? null : props.value}
      onValueChange={(v) => {
        if (typeof v === 'string' && v !== '') props.onValueChange(v)
      }}
    >
      <SelectTrigger className={props.className}>
        <SelectValue placeholder={props.placeholder} />
      </SelectTrigger>
      <SelectContent alignItemWithTrigger={false}>
        <SelectGroup>
          {knownOptions.map((name) => (
            <SelectItem key={name} value={name}>
              {name}
            </SelectItem>
          ))}
        </SelectGroup>
      </SelectContent>
    </Select>
  )
}

type GroupSectionProps = {
  groupName: string
  items: ModelRatioRule[]
  groupOptions: string[]
  availableModels: string[]
  isLoadingPricing: boolean
  onUpdate: (
    id: string,
    field: keyof ModelRatioRule,
    val: string | number
  ) => void
  onRemove: (id: string) => void
  onAdd: (groupName: string) => void
  onRemoveGroup: (groupName: string) => void
  onImportAll: (groupName: string) => void
  onResetAllToOne: (groupName: string) => void
}

function GroupSection(props: GroupSectionProps) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(true)
  const [filterText, setFilterText] = useState('')
  const isKnownGroup = props.groupOptions.includes(props.groupName)

  const datalistId = `datalist-models-${props.groupName}`

  const filteredItems = useMemo(() => {
    if (!filterText.trim()) return props.items
    const lower = filterText.trim().toLowerCase()
    return props.items.filter((item) =>
      item.model.toLowerCase().includes(lower)
    )
  }, [props.items, filterText])

  const discountedCount = useMemo(() => {
    return props.items.filter((item) => {
      const num =
        typeof item.ratio === 'number' ? item.ratio : parseFloat(item.ratio)
      return !isNaN(num) && num < 1.0
    }).length
  }, [props.items])

  return (
    <Collapsible open={open} onOpenChange={setOpen}>
      <datalist id={datalistId}>
        {props.availableModels.map((m) => (
          <option key={m} value={m} />
        ))}
      </datalist>

      <div className='rounded-lg border bg-card text-card-foreground shadow-sm'>
        <div className='flex flex-wrap items-center justify-between gap-2 p-3'>
          <div className='flex items-center gap-2'>
            <CollapsibleTrigger
              render={
                <Button variant='ghost' size='sm' className='h-6 w-6 p-0' />
              }
            >
              {open ? (
                <ChevronUp className='h-4 w-4' />
              ) : (
                <ChevronDown className='h-4 w-4' />
              )}
            </CollapsibleTrigger>
            <span className='font-semibold'>{props.groupName}</span>
            {!isKnownGroup && (
              <StatusBadge variant='danger' copyable={false}>
                <AlertTriangle className='mr-1 h-3 w-3' />
                {t('Not in pricing table')}
              </StatusBadge>
            )}
            <StatusBadge variant='neutral' copyable={false}>
              {props.items.length} {t('models')}
            </StatusBadge>
            {discountedCount > 0 && (
              <StatusBadge variant='info' copyable={false}>
                {discountedCount} {t('discounted (< 1.0)')}
              </StatusBadge>
            )}
          </div>

          <div className='flex items-center gap-1.5'>
            <Button
              variant='outline'
              size='sm'
              className='h-7 text-xs gap-1.5 px-2.5'
              disabled={props.isLoadingPricing}
              title={t('Import all models under this group with default ratio 1.0')}
              onClick={() => props.onImportAll(props.groupName)}
            >
              <Sparkles className='h-3.5 w-3.5 text-primary' />
              <span>{t('获取该分组所有模型 (默认倍率 1)')}</span>
            </Button>

            {props.items.length > 0 && (
              <Button
                variant='ghost'
                size='sm'
                className='h-7 text-xs gap-1 px-2 text-muted-foreground hover:text-foreground'
                title={t('Reset all model ratios in this group to 1.0')}
                onClick={() => props.onResetAllToOne(props.groupName)}
              >
                <RotateCcw className='h-3.5 w-3.5' />
                <span className='hidden sm:inline'>{t('重置为 1.0')}</span>
              </Button>
            )}

            <Button
              variant='ghost'
              size='sm'
              className='h-7 w-7 p-0'
              title={t('Add single model rule')}
              onClick={() => props.onAdd(props.groupName)}
            >
              <Plus className='h-4 w-4' />
            </Button>
            <Button
              variant='ghost'
              size='sm'
              className='text-destructive h-7 w-7 p-0'
              title={t('Delete group')}
              onClick={() => props.onRemoveGroup(props.groupName)}
            >
              <Trash2 className='h-4 w-4' />
            </Button>
          </div>
        </div>

        <CollapsibleContent>
          <div className='space-y-2 border-t p-3'>
            {props.items.length > 5 && (
              <div className='relative mb-2'>
                <Filter className='text-muted-foreground absolute left-2.5 top-2.5 h-3.5 w-3.5' />
                <Input
                  className='h-8 pl-8 text-xs'
                  placeholder={t('Filter models in this group...')}
                  value={filterText}
                  onChange={(e) => setFilterText(e.target.value)}
                />
              </div>
            )}

            {filteredItems.length === 0 ? (
              <p className='text-muted-foreground py-3 text-center text-xs'>
                {props.items.length === 0
                  ? t('No model rules for this group. Click "获取该分组所有模型" or "+" to add.')
                  : t('No models match the filter.')}
              </p>
            ) : (
              filteredItems.map((rule) => {
                const numRatio =
                  typeof rule.ratio === 'number'
                    ? rule.ratio
                    : parseFloat(rule.ratio)
                const isValid = !isNaN(numRatio) && numRatio >= 0

                return (
                  <div key={rule._id} className='flex items-center gap-2'>
                    <div className='flex-1'>
                      <Input
                        list={datalistId}
                        value={rule.model}
                        placeholder={t(
                          'Model name (e.g. gpt-4o, claude-3-5*, *)'
                        )}
                        onChange={(e) =>
                          props.onUpdate(rule._id, 'model', e.target.value)
                        }
                      />
                    </div>
                    <div className='w-[140px]'>
                      <Input
                        type='number'
                        step='0.01'
                        min='0'
                        value={rule.ratio}
                        placeholder={t('Ratio (e.g. 0.8)')}
                        onChange={(e) =>
                          props.onUpdate(rule._id, 'ratio', e.target.value)
                        }
                      />
                    </div>
                    <div className='w-[80px] shrink-0 text-center'>
                      {isValid ? (
                        <StatusBadge
                          variant={numRatio < 1.0 ? 'info' : 'neutral'}
                          copyable={false}
                        >
                          {`${(numRatio * 100).toFixed(0)}%`}
                        </StatusBadge>
                      ) : (
                        <StatusBadge variant='danger' copyable={false}>
                          {t('Invalid')}
                        </StatusBadge>
                      )}
                    </div>
                    <Button
                      variant='ghost'
                      size='sm'
                      className='text-destructive h-8 w-8 p-0'
                      onClick={() => props.onRemove(rule._id)}
                    >
                      <Trash2 className='h-4 w-4' />
                    </Button>
                  </div>
                )
              })
            )}
          </div>
        </CollapsibleContent>
      </div>
    </Collapsible>
  )
}

export type GroupModelRatioVisualEditorProps = {
  value: string
  groupOptions: string[]
  onChange: (value: string) => void
}

export function GroupModelRatioVisualEditor(
  props: GroupModelRatioVisualEditorProps
) {
  const { t } = useTranslation()
  const [rules, setRules] = useState<ModelRatioRule[]>(() =>
    flattenRules(safeParseJson(props.value))
  )

  const { data: pricingRes, isLoading: isLoadingPricing } = useQuery({
    queryKey: ['pricing'],
    queryFn: getPricing,
    staleTime: 5 * 60 * 1000,
  })

  // Get models enabled for a specific group from the pricing catalog
  const getModelsForGroup = useCallback(
    (groupName: string): string[] => {
      if (!pricingRes?.data || !Array.isArray(pricingRes.data)) return []
      const matched = pricingRes.data
        .filter(
          (m) =>
            Array.isArray(m.enable_groups) &&
            m.enable_groups.includes(groupName)
        )
        .map((m) => m.model_name)
      if (matched.length > 0) {
        return Array.from(new Set(matched))
      }
      // Fallback: if group has no specific channel filters, provide all active models
      return Array.from(new Set(pricingRes.data.map((m) => m.model_name)))
    },
    [pricingRes]
  )

  const { onChange, value } = props
  const lastEmittedValueRef = useRef(value)

  useEffect(() => {
    if (value !== lastEmittedValueRef.current) {
      lastEmittedValueRef.current = value
      setRules(flattenRules(safeParseJson(value)))
    }
  }, [value])

  const emitChange = useCallback(
    (newRules: ModelRatioRule[]) => {
      setRules(newRules)
      const serialized = serializeRules(newRules)
      lastEmittedValueRef.current = serialized
      onChange(serialized)
    },
    [onChange]
  )

  const updateRule = useCallback(
    (id: string, field: keyof ModelRatioRule, val: string | number) => {
      emitChange(
        rules.map((r) => {
          if (r._id !== id) return r
          return { ...r, [field]: val }
        })
      )
    },
    [rules, emitChange]
  )

  const removeRule = useCallback(
    (id: string) => emitChange(rules.filter((r) => r._id !== id)),
    [rules, emitChange]
  )

  const removeGroup = useCallback(
    (groupName: string) =>
      emitChange(rules.filter((r) => r.group !== groupName)),
    [rules, emitChange]
  )

  const addRuleToGroup = useCallback(
    (groupName: string) => {
      emitChange([
        ...rules,
        {
          _id: uid(),
          group: groupName,
          model: '',
          ratio: 1.0,
        },
      ])
    },
    [rules, emitChange]
  )

  // One-click import all models belonging to this group with default ratio 1.0
  const importAllGroupModels = useCallback(
    (groupName: string) => {
      const available = getModelsForGroup(groupName)
      if (available.length === 0) {
        toast.info(t('未在当前渠道中检索到该分组的模型，可在下方手动添加'))
        return
      }

      const existingModels = new Set(
        rules.filter((r) => r.group === groupName).map((r) => r.model.trim())
      )

      const toAdd: ModelRatioRule[] = []
      for (const model of available) {
        if (!existingModels.has(model)) {
          toAdd.push({
            _id: uid(),
            group: groupName,
            model,
            ratio: 1.0,
          })
        }
      }

      if (toAdd.length === 0) {
        toast.info(t('当前分组下的所有模型已存在列表中'))
        return
      }

      emitChange([...rules, ...toAdd])
      toast.success(
        t('成功加载 {{count}} 个模型，默认倍率均为 1.0', {
          count: toAdd.length,
        })
      )
    },
    [getModelsForGroup, rules, emitChange, t]
  )

  const resetAllToOne = useCallback(
    (groupName: string) => {
      emitChange(
        rules.map((r) => {
          if (r.group !== groupName) return r
          return { ...r, ratio: 1.0 }
        })
      )
      toast.success(t('已将该分组下的所有模型倍率重置为 1.0'))
    },
    [rules, emitChange, t]
  )

  const grouped = useMemo(() => {
    const map: Record<string, ModelRatioRule[]> = {}
    const order: string[] = []
    for (const r of rules) {
      if (!r.group) continue
      if (!map[r.group]) {
        map[r.group] = []
        order.push(r.group)
      }
      map[r.group].push(r)
    }
    return order.map((name) => ({ name, items: map[name] }))
  }, [rules])

  const newGroupCandidates = useMemo(() => {
    const used = new Set(grouped.map((g) => g.name))
    return props.groupOptions.filter((name) => !used.has(name))
  }, [grouped, props.groupOptions])

  return (
    <Card className={sectionCardClassName}>
      <CardHeader className={sectionHeaderClassName}>
        <div className='flex items-center justify-between'>
          <div>
            <CardTitle>{t('分组模型独立倍率设置 (Group Model Ratios)')}</CardTitle>
            <CardDescription className='mt-1'>
              {t(
                '支持获取当前分组下的所有可用模型，默认倍率为 1.0。针对需要单独打折的模型直接修改倍率即可。'
              )}
            </CardDescription>
          </div>
        </div>
      </CardHeader>
      <CardContent>
        <div className='space-y-3 pt-3'>
          {grouped.length === 0 ? (
            <div className='rounded-lg border border-dashed py-8 text-center'>
              <Sparkles className='mx-auto h-8 w-8 text-muted-foreground/50' />
              <p className='text-muted-foreground mt-2 text-sm'>
                {t('暂无分组模型倍率规则。请在下方选择一个分组进行添加。')}
              </p>
            </div>
          ) : (
            grouped.map((group) => (
              <GroupSection
                key={group.name}
                groupName={group.name}
                items={group.items}
                groupOptions={props.groupOptions}
                availableModels={getModelsForGroup(group.name)}
                isLoadingPricing={isLoadingPricing}
                onUpdate={updateRule}
                onRemove={removeRule}
                onAdd={addRuleToGroup}
                onRemoveGroup={removeGroup}
                onImportAll={importAllGroupModels}
                onResetAllToOne={resetAllToOne}
              />
            ))
          )}

          <div className='flex items-center justify-center pt-2'>
            <GroupSelect
              className='w-[280px]'
              options={newGroupCandidates}
              value=''
              placeholder={t('选择用户分组以配置模型倍率')}
              onValueChange={(selectedGroup) => {
                // When selecting a new group, add it and automatically load its models with default ratio 1.0!
                const available = getModelsForGroup(selectedGroup)
                if (available.length > 0) {
                  const initialRules = available.map((model) => ({
                    _id: uid(),
                    group: selectedGroup,
                    model,
                    ratio: 1.0,
                  }))
                  emitChange([...rules, ...initialRules])
                  toast.success(
                    t('已自动获取分组「{{group}}」下的 {{count}} 个模型，默认倍率均为 1.0', {
                      group: selectedGroup,
                      count: initialRules.length,
                    })
                  )
                } else {
                  addRuleToGroup(selectedGroup)
                }
              }}
            />
          </div>
        </div>
      </CardContent>
    </Card>
  )
}
