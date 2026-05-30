import * as z from 'zod'

// Vee-validate Zod schema for the AI Tool form.
//
// Mirrors the backend validation in cmd/ai_tools.go. Server is the source
// of truth; this only catches the most obvious errors before submit.
export const createToolFormSchema = (t) =>
  z.object({
    name: z
      .string({ required_error: t('globals.messages.required') })
      .min(1, { message: t('globals.messages.required') }),
    slug: z.string().optional().default(''),
    description: z.string().optional().default(''),
    category: z.enum(['read', 'write']).default('write'),
    timeout_ms: z
      .number({ invalid_type_error: t('globals.messages.required') })
      .int()
      .min(1)
      .max(120000)
      .default(30000),
    async: z.boolean().default(false),
    enabled: z.boolean().default(true)
  })
