// Slugs are an internal concern: never user-editable. Generated client-side
// from the title (articles) or folder name (folders) at submit time via
// utils/slugify.js, then validated server-side. The schemas below cover only
// the fields the user sees.
import * as z from 'zod'

export const createFormSchema = (t) =>
  z.object({
    title: z
      .string({ required_error: t('globals.messages.required') })
      .min(1, { message: t('globals.messages.required') })
      .max(500, { message: t('admin.knowledge.title.tooLong') }),
    folder_id: z
      .number({ required_error: t('globals.messages.required') })
      .int()
      .positive({ message: t('admin.knowledge.folder.required') }),
    locale: z.enum(['en', 'de', 'fr'], {
      errorMap: () => ({ message: t('admin.knowledge.locale.invalid') })
    }),
    classification: z.enum(['faq', 'how_to', 'policy', 'news', 'other'], {
      errorMap: () => ({ message: t('admin.knowledge.classification.invalid') })
    }),
    is_ai_groundable: z.boolean(),
    body: z.object({
      html: z.string(),
      json: z.record(z.any()),
      markdown: z.string()
    })
  })

export const createFolderSchema = (t) =>
  z.object({
    name: z
      .string({ required_error: t('globals.messages.required') })
      .min(1, { message: t('globals.messages.required') })
      .max(255)
  })
