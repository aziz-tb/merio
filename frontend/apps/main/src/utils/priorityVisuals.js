import { HighestColored, LowestColored, TrivialColored, MediumColored, CriticalColored } from '@qall/icons'

export const PRIORITY_VISUALS = {
  Low: { icon: 'LowestColored' },
  Medium: { icon: 'MediumColored' },
  High: { icon: 'HighestColored' },
  Urgent: { icon: 'CriticalColored' }
}

export const FALLBACK_PRIORITY_VISUAL = {
  icon: 'TrivialColored'
}

export const PRIORITY_ICONS = {
  LowestColored,
  MediumColored,
  HighestColored,
  CriticalColored,
  TrivialColored
}

export function getPriorityVisual (priorityName) {
  return PRIORITY_VISUALS[priorityName] || FALLBACK_PRIORITY_VISUAL
}
