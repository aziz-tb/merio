import { cn } from '@shared-ui/lib/utils'
import {
  AlarmClock,
  Archive,
  BadgeCheck,
  CalendarClock,
  CheckCheck,
  CircleAlert,
  CircleCheck,
  CircleDashed,
  CircleDot,
  CircleEllipsis,
  CirclePause,
  CircleSlash,
  CircleX,
  Clock,
  Flag,
  Hourglass,
  Inbox,
  ListChecks,
  MailOpen,
  MessageCircle,
  MessageCircleReply,
  Moon,
  ShieldCheck,
  TicketCheck,
  Timer
} from 'lucide-vue-next'
import { CONVERSATION_DEFAULT_STATUSES } from '@main/constants/conversation'
import { CircleCheckBig } from 'lucide-vue-next'

export const STATUS_COLORS = ['grey', 'blue', 'green', 'yellow', 'orange', 'red', 'purple', 'teal']

export const STATUS_COLOR_CLASSES = {
  grey: 'border-border bg-secondary text-secondary-foreground border-secondary-foreground/20 dark:border-secondary-700',
  blue: 'border-blue-100 bg-blue-100/70 text-blue-900 dark:border-blue-800 dark:bg-blue-950 dark:text-blue-100',
  green: 'border-green-300 bg-green-300/50 text-green-700 dark:border-green-800 dark:bg-green-950 dark:text-green-200',
  yellow: 'border-yellow-300 bg-yellow-100 text-yellow-900 dark:border-yellow-800 dark:bg-yellow-950 dark:text-yellow-200',
  orange: 'border-orange-200 bg-orange-100 text-orange-900 dark:border-orange-800 dark:bg-orange-950 dark:text-orange-100',
  red: 'border-red-200 bg-red-100 text-red-900 dark:border-red-800 dark:bg-red-950 dark:text-red-100',
  purple: 'border-purple-200 bg-purple-100 text-purple-900 dark:border-purple-800 dark:bg-purple-950 dark:text-purple-100',
  teal: 'border-teal-200 bg-teal-100 text-teal-900 dark:border-teal-800 dark:bg-teal-950 dark:text-teal-100'
}

export const STATUS_COLOR_DOT_CLASSES = {
  grey: 'bg-secondary-foreground/60',
  blue: 'bg-blue-500',
  green: 'bg-green-500',
  yellow: 'bg-yellow-500',
  orange: 'bg-orange-500',
  red: 'bg-red-500',
  purple: 'bg-purple-500',
  teal: 'bg-teal-500'
}

export const STATUS_ICONS = {
  CircleDot,
  CircleDashed,
  CircleEllipsis,
  CircleAlert,
  CircleCheckBig,
  CircleSlash,
  CircleX,
  CirclePause,
  Moon,
  Clock,
  Timer,
  Hourglass,
  AlarmClock,
  CalendarClock,
  Inbox,
  Archive,
  MailOpen,
  MessageCircle,
  MessageCircleReply,
  Flag,
  BadgeCheck,
  ShieldCheck,
  CheckCheck,
  ListChecks,
  TicketCheck
}

export const STATUS_ICON_OPTIONS = Object.keys(STATUS_ICONS).map((icon) => ({
  label: icon,
  value: icon,
  icon
}))

export const FALLBACK_STATUS_VISUAL = {
  color: 'blue',
  icon: 'CircleDot'
}

export const DEFAULT_STATUS_VISUALS = {
  [CONVERSATION_DEFAULT_STATUSES.OPEN]: {
    color: 'blue',
    icon: 'CircleDot'
  },
  [CONVERSATION_DEFAULT_STATUSES.SNOOZED]: {
    color: 'yellow',
    icon: 'Moon'
  },
  [CONVERSATION_DEFAULT_STATUSES.RESOLVED]: {
    color: 'green',
    icon: 'CircleCheckBig'
  },
  [CONVERSATION_DEFAULT_STATUSES.CLOSED]: {
    color: 'grey',
    icon: 'CircleSlash'
  }
}

export const DEFAULT_STATUS_LABEL_KEYS = {
  [CONVERSATION_DEFAULT_STATUSES.OPEN]: 'globals.terms.open',
  [CONVERSATION_DEFAULT_STATUSES.SNOOZED]: 'globals.terms.snoozed',
  [CONVERSATION_DEFAULT_STATUSES.RESOLVED]: 'globals.terms.resolved',
  [CONVERSATION_DEFAULT_STATUSES.CLOSED]: 'globals.terms.closed'
}

export const CATEGORY_STATUS_VISUALS = {
  open: {
    color: 'blue',
    icon: 'CircleDot'
  },
  waiting: {
    color: 'yellow',
    icon: 'Moon'
  },
  resolved: {
    color: 'green',
    icon: 'CircleCheckBig'
  }
}

export const STATUS_PILL_BASE_CLASS = 'inline-flex max-w-full items-center gap-1 rounded-full border font-medium'

export const STATUS_PILL_SIZE_CLASSES = {
  xs: 'min-h-5 px-2 py-0.5 text-[11px]',
  sm: 'min-h-5 px-2 py-1.5 text-xs',
  md: 'h-8 px-2 text-xs'
}

export const STATUS_ICON_SIZE_CLASSES = {
  xs: 'size-3',
  sm: 'size-3',
  md: 'size-3.5'
}

export function isDefaultStatus(status) {
  return Boolean(DEFAULT_STATUS_VISUALS[getStatusName(status)])
}

export function getStatusName(status) {
  if (!status) return ''
  if (typeof status === 'string') return status
  return status.name || status.label || ''
}

export function getStatusDisplayLabel(status, t) {
  const name = getStatusName(status)
  const key = DEFAULT_STATUS_LABEL_KEYS[name]
  return key && typeof t === 'function' ? t(key) : name
}

export function normalizeStatusColor(color) {
  return STATUS_COLORS.includes(color) ? color : FALLBACK_STATUS_VISUAL.color
}

export function normalizeStatusIcon(icon) {
  return STATUS_ICONS[icon] ? icon : FALLBACK_STATUS_VISUAL.icon
}

export function getStatusVisual(status) {
  const name = getStatusName(status)
  if (DEFAULT_STATUS_VISUALS[name]) {
    return DEFAULT_STATUS_VISUALS[name]
  }

  const categoryVisual = CATEGORY_STATUS_VISUALS[status?.category] || FALLBACK_STATUS_VISUAL
  return {
    color: normalizeStatusColor(status?.color || categoryVisual.color),
    icon: normalizeStatusIcon(status?.icon || categoryVisual.icon)
  }
}

export function getStatusColorClass(color) {
  return STATUS_COLOR_CLASSES[normalizeStatusColor(color)]
}

export function getStatusColorDotClass(color) {
  return STATUS_COLOR_DOT_CLASSES[normalizeStatusColor(color)]
}

export function getStatusPillClass(color, options = {}) {
  return cn(
    STATUS_PILL_BASE_CLASS,
    STATUS_PILL_SIZE_CLASSES[options.size] || STATUS_PILL_SIZE_CLASSES.sm,
    getStatusColorClass(color),
    options.class
  )
}
