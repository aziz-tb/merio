/**
 * @typedef {Object} ArticleBody
 * @property {string} html      Output of editor.getHTML()
 * @property {Object} json      Output of editor.getJSON() (TipTap doc tree)
 * @property {string} markdown  Output of editor.storage.markdown.getMarkdown() (tiptap-markdown)
 */

/**
 * Synchronously read all three body formats from a TipTap editor instance.
 * Submit handlers MUST call this directly rather than relying on the
 * debounced v-model — otherwise characters typed in the final 200ms
 * before submit can be lost.
 *
 * @param {import('@tiptap/vue-3').Editor} editor
 * @returns {ArticleBody}
 */
export function getBody(editor) {
  if (!editor) {
    return { html: '', json: { type: 'doc', content: [] }, markdown: '' }
  }
  const html = editor.getHTML()
  const json = editor.getJSON()
  const markdown =
    editor.storage && editor.storage.markdown
      ? editor.storage.markdown.getMarkdown()
      : ''
  return { html, json, markdown }
}

/**
 * Returns true when the body shape has all three required fields.
 * Used by formSchema as a structural sanity check before zod-typed
 * validation kicks in.
 *
 * @param {unknown} body
 * @returns {boolean}
 */
export function isValidBody(body) {
  return (
    body !== null &&
    typeof body === 'object' &&
    typeof body.html === 'string' &&
    typeof body.json === 'object' &&
    body.json !== null &&
    typeof body.markdown === 'string'
  )
}
