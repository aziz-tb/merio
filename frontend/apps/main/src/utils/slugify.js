/**
 * Canonical slug derivation, used by Knowledge folders/articles, AI agents,
 * AI tools, and any future entity that wants a slug field. Must stay in
 * sync with the Go-side `internal/stringutil/Slugify` — both implementations
 * are unit-tested against the same fixtures.
 *
 * Pipeline:
 *   1. NFKD-normalize and strip combining marks. Accented Latin chars
 *      fold to ASCII (Café → Cafe).
 *   2. Insert dashes at camelCase boundaries (echoTwoFields →
 *      echo-Two-Fields), including acronym runs (HTTPRequest →
 *      HTTP-Request).
 *   3. Lowercase.
 *   4. Replace every run of non-[a-z0-9] with a single dash.
 *   5. Trim leading/trailing dashes.
 *
 * Examples:
 *   slugify("echoTwoFields")    === "echo-two-fields"
 *   slugify("Café Faktura")     === "cafe-faktura"
 *   slugify("My Tool!! Foo")    === "my-tool-foo"
 *   slugify("HTTPRequest")      === "http-request"
 *   slugify("set_meter_reading") === "set-meter-reading"
 *
 * @param {string} input
 * @returns {string}
 */
export function slugify(input) {
  if (typeof input !== 'string') return ''
  return input
    .normalize('NFKD')
    .replace(/[̀-ͯ]/g, '') // strip combining marks
    .replace(/([A-Z]+)([A-Z][a-z])/g, '$1-$2') // acronym run
    .replace(/([a-z0-9])([A-Z])/g, '$1-$2') // camelCase boundary
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
}
