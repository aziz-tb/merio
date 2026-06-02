import { cn } from '../lib/utils'

export const TAG_COLORS = ['grey', 'green', 'red', 'blue', 'yellow', 'purple', 'orange', 'teal']

export const TAG_COLOR_CLASSES = {
  grey: 'border-border bg-secondary text-secondary-foreground',
  green: 'border-green-200 bg-green-100 text-green-900 dark:border-green-800 dark:bg-green-950 dark:text-green-100',
  red: 'border-red-200 bg-red-100 text-red-900 dark:border-red-800 dark:bg-red-950 dark:text-red-100',
  blue: 'border-blue-200 bg-blue-100 text-blue-900 dark:border-blue-800 dark:bg-blue-950 dark:text-blue-100',
  yellow: 'border-yellow-200 bg-yellow-100 text-yellow-900 dark:border-yellow-800 dark:bg-yellow-950 dark:text-yellow-100',
  purple: 'border-purple-200 bg-purple-100 text-purple-900 dark:border-purple-800 dark:bg-purple-950 dark:text-purple-100',
  orange: 'border-orange-200 bg-orange-100 text-orange-900 dark:border-orange-800 dark:bg-orange-950 dark:text-orange-100',
  teal: 'border-teal-200 bg-teal-100 text-teal-900 dark:border-teal-800 dark:bg-teal-950 dark:text-teal-100'
}

export const TAG_PILL_BASE_CLASS = 'inline-flex max-w-full items-center rounded-md border font-medium'

export const TAG_PILL_SIZE_CLASSES = {
  xs: 'min-h-5 px-2 py-0.5 text-[11px]',
  sm: 'min-h-5 px-2 py-0.5 text-xs',
  input: 'min-h-5 text-sm'
}

export function normalizeTagColor(color) {
  return TAG_COLORS.includes(color) ? color : 'grey'
}

export function getTagColorClass(color) {
  return TAG_COLOR_CLASSES[normalizeTagColor(color)]
}

export function getTagPillClass(color, options = {}) {
  return cn(
    TAG_PILL_BASE_CLASS,
    TAG_PILL_SIZE_CLASSES[options.size] || TAG_PILL_SIZE_CLASSES.sm,
    getTagColorClass(color),
    options.class
  )
}
