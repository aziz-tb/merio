import { describe, test, expect } from 'vitest'
import { getThumbFilepath } from './file'

describe('getThumbFilepath', () => {
    test('returns falsy input unchanged', () => {
        expect(getThumbFilepath('')).toBe('')
        expect(getThumbFilepath(null)).toBe(null)
        expect(getThumbFilepath(undefined)).toBe(undefined)
    })

    test('builds thumb path from relative fs upload path', () => {
        expect(getThumbFilepath('/uploads/9a4f0a03-7b36-4e05-aacd-4eb04947a79d'))
            .toBe('/uploads/thumb_9a4f0a03-7b36-4e05-aacd-4eb04947a79d')
    })

    test('preserves signature params from signed fs url', () => {
        const signed = 'https://example.com/uploads/9a4f0a03-7b36-4e05-aacd-4eb04947a79d?sig=abc&exp=1700000000'
        expect(getThumbFilepath(signed))
            .toBe('/uploads/thumb_9a4f0a03-7b36-4e05-aacd-4eb04947a79d?sig=abc&exp=1700000000')
    })

    test('preserves query string from presigned s3 url', () => {
        const presigned = 'https://s3.ap-south-1.amazonaws.com/example-bucket/9a4f0a03-7b36-4e05-aacd-4eb04947a79d?X-Amz-Algorithm=AWS4-HMAC-SHA256&X-Amz-Signature=deadbeef'
        expect(getThumbFilepath(presigned))
            .toBe('/uploads/thumb_9a4f0a03-7b36-4e05-aacd-4eb04947a79d?X-Amz-Algorithm=AWS4-HMAC-SHA256&X-Amz-Signature=deadbeef')
    })

    test('returns original url when no uuid found', () => {
        expect(getThumbFilepath('https://example.com/some/path'))
            .toBe('https://example.com/some/path')
    })
})
