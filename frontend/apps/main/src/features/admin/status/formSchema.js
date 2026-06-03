import * as z from 'zod'
import { STATUS_COLORS, STATUS_ICON_OPTIONS } from '@main/utils/statusVisuals'

const STATUS_ICON_VALUES = STATUS_ICON_OPTIONS.map((option) => option.value)

export const createFormSchema = (t) => z.object({
  name: z
    .string({
      required_error: t('globals.messages.required'),
    })
    .min(1, {
      message: t('validation.minmax', {
        min: 1,
        max: 25,
      })
    })
    .max(25, {
      message: t('validation.minmax', {
        min: 1,
        max: 25,
      })
    }),
  category: z.enum(['open', 'waiting', 'resolved'], {
    required_error: t('globals.messages.required'),
  }),
  color: z.enum(STATUS_COLORS).default('blue'),
  icon: z.enum(STATUS_ICON_VALUES).default('CircleDot')
})
