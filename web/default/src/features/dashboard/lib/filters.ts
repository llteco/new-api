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
  getNaturalDateRange,
  getRollingDateRange,
  type TimeGranularity,
} from '@/lib/time'
import {
  DASHBOARD_CHART_PREFERENCES_STORAGE_KEY,
  DEFAULT_DASHBOARD_CHART_PREFERENCES,
  DEFAULT_TIME_GRANULARITY,
  EMPTY_DASHBOARD_FILTERS,
  TIME_GRANULARITY_STORAGE_KEY,
  TIME_RANGE_PRESETS,
  TIME_RANGE_BY_GRANULARITY,
  type TimeRangePreset,
  type TimeRangePresetKey,
} from '@/features/dashboard/constants'
import type {
  ConsumptionDistributionChartType,
  DashboardChartPreferences,
  DashboardFilters,
  ModelAnalyticsChartTab,
} from '@/features/dashboard/types'

function isTimeGranularity(value: unknown): value is TimeGranularity {
  return value === 'hour' || value === 'day' || value === 'week'
}

function getLegacySavedGranularity(): TimeGranularity {
  if (typeof window === 'undefined') return DEFAULT_TIME_GRANULARITY
  const saved = localStorage.getItem(TIME_GRANULARITY_STORAGE_KEY)
  return isTimeGranularity(saved) ? saved : DEFAULT_TIME_GRANULARITY
}

function isConsumptionDistributionChartType(
  value: unknown
): value is ConsumptionDistributionChartType {
  return value === 'bar' || value === 'area'
}

function isModelAnalyticsChartTab(
  value: unknown
): value is ModelAnalyticsChartTab {
  return value === 'trend' || value === 'proportion' || value === 'top'
}

function isTimeRangePresetKey(value: unknown): value is TimeRangePresetKey {
  return TIME_RANGE_PRESETS.some((preset) => preset.key === value)
}

export function cleanFilters<T extends Record<string, unknown>>(
  filters: T
): Partial<T> {
  const cleaned: Partial<T> = {}
  for (const [key, value] of Object.entries(filters)) {
    if (value === undefined || value === null) continue
    if (typeof value === 'string') {
      const trimmed = value.trim()
      if (trimmed) cleaned[key as keyof T] = trimmed as T[keyof T]
      continue
    }
    cleaned[key as keyof T] = value as T[keyof T]
  }
  return cleaned
}

export function getSavedGranularity(
  override?: TimeGranularity
): TimeGranularity {
  if (override) return override
  return getSavedChartPreferences().defaultTimeGranularity
}

export function saveGranularity(granularity: TimeGranularity): void {
  if (typeof window === 'undefined') return
  saveChartPreferences({
    ...getSavedChartPreferences(),
    defaultTimeGranularity: granularity,
  })
  localStorage.setItem(TIME_GRANULARITY_STORAGE_KEY, granularity)
}

export function getSavedChartPreferences(): DashboardChartPreferences {
  if (typeof window === 'undefined') return DEFAULT_DASHBOARD_CHART_PREFERENCES

  const fallbackPreferences = {
    ...DEFAULT_DASHBOARD_CHART_PREFERENCES,
    defaultTimeGranularity: getLegacySavedGranularity(),
  }

  try {
    const raw = localStorage.getItem(DASHBOARD_CHART_PREFERENCES_STORAGE_KEY)
    if (!raw) return fallbackPreferences

    const parsed = JSON.parse(raw) as Partial<DashboardChartPreferences>
    return {
      consumptionDistributionChart: isConsumptionDistributionChartType(
        parsed.consumptionDistributionChart
      )
        ? parsed.consumptionDistributionChart
        : fallbackPreferences.consumptionDistributionChart,
      modelAnalyticsChart: isModelAnalyticsChartTab(parsed.modelAnalyticsChart)
        ? parsed.modelAnalyticsChart
        : fallbackPreferences.modelAnalyticsChart,
      defaultTimeRangeDays: isTimeRangePresetKey(parsed.defaultTimeRangeDays)
        ? parsed.defaultTimeRangeDays
        : (fallbackPreferences.defaultTimeRangeDays as TimeRangePresetKey),
      defaultTimeGranularity: isTimeGranularity(parsed.defaultTimeGranularity)
        ? parsed.defaultTimeGranularity
        : fallbackPreferences.defaultTimeGranularity,
    }
  } catch {
    return fallbackPreferences
  }
}

export function saveChartPreferences(
  preferences: DashboardChartPreferences
): void {
  if (typeof window === 'undefined') return
  localStorage.setItem(
    DASHBOARD_CHART_PREFERENCES_STORAGE_KEY,
    JSON.stringify(preferences)
  )
}

export function getDefaultDays(granularity?: TimeGranularity): TimeRangePresetKey {
  if (!granularity) return getSavedChartPreferences().defaultTimeRangeDays
  return TIME_RANGE_BY_GRANULARITY[getSavedGranularity(granularity)]
}

// Helper for callers that still need a numeric day count, e.g. to
// feed computeTimeRange. Returns the rolling-day value of the preset
// (or a sensible fallback for natural ranges so a missing custom range
// still produces a finite window).
export function getPresetRollingDays(key: TimeRangePresetKey): number {
  const preset = getPresetByKey(key)
  if (preset.kind === 'rolling') return preset.days
  // For natural calendar ranges, fall back to a rolling window that
  // roughly matches the period so the legacy helper has a finite
  // default if no custom start/end is supplied.
  switch (preset.range) {
    case 'thisMonth':
    case 'lastMonth':
      return 30
    case 'thisYear':
      return 365
    default:
      // lastYear or any future natural range without an explicit case.
      return 365
  }
}

export function getPresetByKey(key: TimeRangePresetKey): TimeRangePreset {
  const preset = TIME_RANGE_PRESETS.find((p) => p.key === key)
  if (preset) {
    return preset
  }
  // Should not happen if the caller passes a valid key; fall back to
  // the first preset (always a rolling 1-day window) so we never
  // return undefined and the UI keeps rendering.
  return TIME_RANGE_PRESETS[0]
}

export function getPresetDateRange(
  key: TimeRangePresetKey
): { start: Date; end: Date } {
  const preset = getPresetByKey(key)
  if (preset.kind === 'natural') {
    return getNaturalDateRange(preset.range)
  }
  return getRollingDateRange(preset.days)
}

export function buildDefaultDashboardFilters(
  preferences: DashboardChartPreferences = getSavedChartPreferences()
): DashboardFilters {
  const { start, end } = getPresetDateRange(preferences.defaultTimeRangeDays)
  return {
    ...EMPTY_DASHBOARD_FILTERS,
    start_timestamp: start,
    end_timestamp: end,
    time_granularity: preferences.defaultTimeGranularity,
  }
}

export function buildQueryParams(
  timeRange: { start_timestamp: number; end_timestamp: number },
  filters?: { time_granularity?: TimeGranularity; username?: string }
): {
  start_timestamp: number
  end_timestamp: number
  default_time: string
  username?: string
} {
  return {
    ...timeRange,
    default_time: getSavedGranularity(filters?.time_granularity),
    ...(filters?.username && { username: filters.username }),
  }
}
