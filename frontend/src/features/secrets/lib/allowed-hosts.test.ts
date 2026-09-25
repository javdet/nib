import { describe, expect, it } from 'vitest'
import { addsAllowedHost, parseAllowedHosts } from './allowed-hosts'

describe('parseAllowedHosts', () => {
	it('splits on lines, commas and spaces', () => {
		expect(parseAllowedHosts('b.example.com\na.example.com, kb  c.example.com')).toEqual([
			'a.example.com',
			'b.example.com',
			'c.example.com',
			'kb',
		])
	})

	it('lowercases, strips a trailing dot and brackets, and deduplicates', () => {
		expect(parseAllowedHosts('API.GitHub.com.\napi.github.com\n[::1]')).toEqual([
			'::1',
			'api.github.com',
		])
	})

	it('returns an empty list for blank input', () => {
		expect(parseAllowedHosts('  \n ')).toEqual([])
	})
})

describe('addsAllowedHost', () => {
	it('is true when a host is new', () => {
		expect(addsAllowedHost(['api.github.com'], ['api.github.com', 'evil.test'])).toBe(true)
	})

	it('is false when hosts are only removed or kept', () => {
		expect(addsAllowedHost(['a', 'b'], ['a'])).toBe(false)
		expect(addsAllowedHost(['a'], ['a'])).toBe(false)
		expect(addsAllowedHost(['a'], [])).toBe(false)
	})
})
