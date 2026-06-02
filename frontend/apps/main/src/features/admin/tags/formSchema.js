import * as z from 'zod'
import { TAG_COLORS } from '@shared-ui/utils/tag'

export const createFormSchema = (t) => z.object({
  name: z
    .string({
      required_error: t('globals.messages.required'),
    })
    .min(3, {
      message: t('admin.conversationTags.name.valid'),
    }),
  parent_id: z.any().nullable().optional(),
  type: z.enum(['skill', 'topic', 'contact', 'other']).default('skill'),
  color: z.enum(TAG_COLORS).default('grey'),
  visibility: z.enum(['public', 'private', 'disabled']).default('public'),
  description: z.string().default(''),
  detection: z.any().optional(),
  sla_policy_id: z.any().nullable().optional(),
  priority_id: z.any().nullable().optional()
})
