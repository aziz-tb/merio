import * as z from 'zod'

// Vee-validate Zod schema for the AI Agent form.
//
// Mirrors the backend validation in cmd/ai_agents.go but stays minimal:
// the server is the source of truth, this only catches the most obvious
// errors before submit so the user gets fast feedback.
export const createAgentFormSchema = (t) =>
  z
    .object({
      name: z
        .string({ required_error: t('globals.messages.required') })
        .min(1, { message: t('globals.messages.required') }),
      slug: z.string().optional().default(''),
      description: z.string().optional().default(''),
      prompt: z.string().optional().default(''),
      // preset_overrides is JSON-string-edited via a JsonField (mirroring
      // tools/detection/autonomy). It lives in a local ref on the form
      // component, NOT in vee-validate state, so it doesn't appear in this
      // schema. Submit is gated on a parse-error ref.
      confidence_threshold: z
        .number({ invalid_type_error: t('globals.messages.required') })
        .min(0)
        .max(1)
        .default(0.5),
      extended_thinking_enabled: z.boolean().default(true),
      max_iterations: z
        .number({ invalid_type_error: t('globals.messages.required') })
        .int()
        .min(1)
        .max(50)
        .default(5),
      enabled: z.boolean().default(true),
      min_verification: z
        .number({ invalid_type_error: t('globals.messages.required') })
        .int()
        .min(0)
        .max(3)
        .default(0),
      kb_scope: z
        .object({
          folder_ids: z.array(z.number().int().positive()).default([]),
          article_ids: z.array(z.number().int().positive()).default([])
        })
        .default({ folder_ids: [], article_ids: [] })
    })
    .superRefine((data, ctx) => {
      // Match backend rule: prompt required when enabled.
      if (data.enabled && (!data.prompt || data.prompt.trim() === '')) {
        ctx.addIssue({
          path: ['prompt'],
          message: t('admin.ai.agent.promptHelp'),
          code: z.ZodIssueCode.custom
        })
      }
    })
