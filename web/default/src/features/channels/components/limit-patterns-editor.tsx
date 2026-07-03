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
import { Plus, Trash2 } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  NativeSelect,
  NativeSelectOption,
} from '@/components/ui/native-select'

import { getLimitPatternPresets } from '../api'
import {
  PREDEFINED_DATE_LAYOUTS,
  validateLimitPatternRegex,
} from '../lib/limit-pattern-utils'
import type { LimitPattern } from '../types'

type LimitPatternsEditorProps = {
  value: LimitPattern[]
  onChange: (value: LimitPattern[]) => void
}

const isPredefinedLayout = (layout: string) =>
  PREDEFINED_DATE_LAYOUTS.some((entry) => entry.value === layout)

export function LimitPatternsEditor(props: LimitPatternsEditorProps) {
  const { t } = useTranslation()
  const [presets, setPresets] = useState<LimitPattern[]>([])
  const keysRef = useRef<string[]>([])
  const keySeedRef = useRef(0)
  const nextKey = () => `lp-${(keySeedRef.current += 1)}`

  // Align synthetic row keys to the current value length so external value
  // changes (e.g. a saved channel being loaded by the parent) stay in sync.
  if (keysRef.current.length !== props.value.length) {
    const aligned = keysRef.current.slice(0, props.value.length)
    while (aligned.length < props.value.length) aligned.push(nextKey())
    keysRef.current = aligned
  }

  const emit = (nextValue: LimitPattern[], nextKeys: string[]) => {
    keysRef.current = nextKeys
    props.onChange(nextValue)
  }

  useEffect(() => {
    getLimitPatternPresets()
      .then(setPresets)
      .catch(() => setPresets([]))
  }, [])

  const updatePattern = (index: number, patch: Partial<LimitPattern>) => {
    emit(
      props.value.map((pattern, i) =>
        i === index ? { ...pattern, ...patch } : pattern
      ),
      keysRef.current
    )
  }

  const removePattern = (index: number) => {
    emit(
      props.value.filter((_, i) => i !== index),
      keysRef.current.filter((_, i) => i !== index)
    )
  }

  const loadPreset = (presetName: string) => {
    const preset = presets.find((pattern) => pattern.name === presetName)
    if (preset) {
      emit([...props.value, { ...preset }], [...keysRef.current, nextKey()])
    }
  }

  const addPattern = () => {
    emit(
      [
        ...props.value,
        {
          name: '',
          regex: '',
          date_layout: '2006-01-02 15:04:05',
          default_minutes: 10,
        },
      ],
      [...keysRef.current, nextKey()]
    )
  }

  return (
    <div className='space-y-4'>
      <div className='flex items-center justify-between gap-3'>
        <h4 className='text-sm font-medium'>
          {t('Limit Detection Patterns')}
        </h4>
        {presets.length > 0 && (
          <NativeSelect
            size='sm'
            defaultValue=''
            onChange={(e) => loadPreset(e.target.value)}
          >
            <NativeSelectOption value='' disabled>
              {t('Load preset')}
            </NativeSelectOption>
            {presets.map((preset) => (
              <NativeSelectOption key={preset.name} value={preset.name}>
                {preset.name}
              </NativeSelectOption>
            ))}
          </NativeSelect>
        )}
      </div>

      {props.value.map((pattern, index) => {
        const validation = validateLimitPatternRegex(pattern.regex)
        const usingCustomLayout = !isPredefinedLayout(pattern.date_layout)
        return (
          <div key={keysRef.current[index]} className='space-y-2 rounded-lg border p-4'>
            <Input
              value={pattern.name}
              placeholder={t('Pattern name')}
              onChange={(e) => updatePattern(index, { name: e.target.value })}
            />
            <Input
              value={pattern.regex}
              placeholder={t('Regex with (?P<reset>...) group')}
              className='font-mono text-sm'
              onChange={(e) => updatePattern(index, { regex: e.target.value })}
              aria-invalid={!validation.valid}
            />
            {!validation.valid && (
              <p className='text-destructive text-sm'>
                {t(validation.error ?? '')}
              </p>
            )}
            <div className='flex gap-2'>
              <NativeSelect
                size='sm'
                value={usingCustomLayout ? 'custom' : pattern.date_layout}
                onChange={(e) => {
                  const next = e.target.value
                  updatePattern(index, {
                    date_layout: next === 'custom' ? '' : next,
                  })
                }}
              >
                {PREDEFINED_DATE_LAYOUTS.map((layout) => (
                  <NativeSelectOption
                    key={layout.value}
                    value={layout.value}
                  >
                    {layout.label}
                  </NativeSelectOption>
                ))}
              </NativeSelect>
              {usingCustomLayout && (
                <Input
                  value={pattern.date_layout}
                  placeholder={t('Custom date layout')}
                  className='flex-1'
                  onChange={(e) =>
                    updatePattern(index, { date_layout: e.target.value })
                  }
                />
              )}
              <Input
                type='number'
                min={1}
                value={pattern.default_minutes}
                className='w-24'
                onChange={(e) =>
                  updatePattern(index, {
                    default_minutes: Number(e.target.value),
                  })
                }
              />
            </div>
            <div>
              <Button
                type='button'
                variant='destructive'
                size='sm'
                onClick={() => removePattern(index)}
              >
                <Trash2 className='size-4' aria-hidden='true' />
                {t('Delete')}
              </Button>
            </div>
          </div>
        )
      })}

      <Button
        type='button'
        variant='outline'
        size='sm'
        className='w-full'
        onClick={addPattern}
      >
        <Plus className='size-4' aria-hidden='true' />
        {t('Add pattern')}
      </Button>
    </div>
  )
}
