/**
 * Hand-rolled slug derivation. Lowercases, replaces every run of
 * non-alphanumerics with `-`, trims leading/trailing dashes. Idempotent
 * on already-slugged input. No external dependency.
 *
 * Note on unicode: input is normalized via NFKD and combining marks are
 * stripped, so accented Latin characters become their ASCII equivalents
 * (`é` → `e`). Non-Latin scripts collapse to empty and the slug becomes
 * `-` after trimming, which the server-side `^[a-z0-9-]+$` validator
 * rejects — content authors then type a slug manually.
 *
 * @param {string} input
 * @returns {string}
 */
export function slugify(input) {
  if (typeof input !== 'string') return ''
  return input
    .normalize('NFKD')
    .replace(/[̀-ͯ]/g, '')
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
}
