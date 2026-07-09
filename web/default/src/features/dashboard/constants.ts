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
import type {
  NaturalRangeKind,
  TimeGranularity,
} from '@/lib/time'
import type { DashboardChartPreferences, DashboardFilters } from './types'

export const TIME_GRANULARITY_STORAGE_KEY = 'data_export_default_time'
export const DASHBOARD_CHART_PREFERENCES_STORAGE_KEY =
  'dashboard_models_chart_preferences'
export const DEFAULT_TIME_GRANULARITY = 'hour' as const
export const MAX_CHART_TREND_POINTS = 7

export const DEFAULT_DASHBOARD_CHART_PREFERENCES: DashboardChartPreferences = {
  consumptionDistributionChart: 'bar',
  modelAnalyticsChart: 'trend',
  defaultTimeRangeDays: 'today',
  defaultTimeGranularity: DEFAULT_TIME_GRANULARITY,
}

export const TIME_RANGE_BY_GRANULARITY: Record<TimeGranularity, TimeRangePresetKey> =
  {
    hour: 'today',
    day: '7d',
    week: 'thisMonth',
  } as const

export const TIME_GRANULARITY_OPTIONS = [
  { label: 'Hour', value: 'hour' },
  { label: 'Day', value: 'day' },
  { label: 'Week', value: 'week' },
] as const

// Range presets that the user can pick on the dashboard charts. The
// kind field discriminates between rolling windows (anchored to now)
// and natural calendar periods (anchored to month/year boundaries).
// The `key` field is the stable identifier stored in user
// preferences; it is independent of the human-readable label so
// translations can change without breaking saved state.
export type TimeRangePresetKey =
  | 'today'
  | '7d'
  | '14d'
  | 'thisMonth'
  | 'lastMonth'
  | 'thisYear'

export type TimeRangePresetKind = 'rolling' | 'natural'

export interface RollingTimeRangePreset {
  kind: 'rolling'
  key: TimeRangePresetKey
  label: string
  days: number
}

export interface NaturalTimeRangePreset {
  kind: 'natural'
  key: TimeRangePresetKey
  label: string
  range: NaturalRangeKind
}

export type TimeRangePreset = RollingTimeRangePreset | NaturalTimeRangePreset

export const TIME_RANGE_PRESETS: readonly TimeRangePreset[] = [
  { kind: 'natural', key: 'today', label: 'Today', range: 'today' },
  { kind: 'rolling', key: '7d', label: '7 Days', days: 7 },
  { kind: 'rolling', key: '14d', label: '14 Days', days: 14 },
  { kind: 'natural', key: 'thisMonth', label: 'This Month', range: 'thisMonth' },
  { kind: 'natural', key: 'lastMonth', label: 'Last Month', range: 'lastMonth' },
  { kind: 'natural', key: 'thisYear', label: 'This Year', range: 'thisYear' },
] as const

export const CONSUMPTION_DISTRIBUTION_CHART_OPTIONS = [
  { value: 'bar', labelKey: 'Bar Chart' },
  { value: 'area', labelKey: 'Area Chart' },
] as const

export const MODEL_ANALYTICS_CHART_OPTIONS = [
  { value: 'trend', labelKey: 'Call Trend' },
  { value: 'proportion', labelKey: 'Call Count Distribution' },
  { value: 'top', labelKey: 'Call Count Ranking' },
] as const

export const EMPTY_DASHBOARD_FILTERS: DashboardFilters = {
  start_timestamp: undefined,
  end_timestamp: undefined,
  time_granularity: 'hour',
  username: '',
}
