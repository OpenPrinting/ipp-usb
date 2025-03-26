/* ipp-usb - HTTP reverse proxy, backed by IPP-over-USB connection to device
 *
 * Copyright (C) 2020 and up by Alexander Pevzner (pzz@apevzner.com)
 * See LICENSE for license terms and conditions
 *
 * Glob-style pattern matching
 */

package main

// GlobMatch matches string against glob-style pattern.
// Pattern  may contain wildcards and has a following syntax:
//
//	?   - matches exactly one character
//	*   - matches any sequence of characters
//	\C  - matches character C
//	C   - matches character C (C is not *, ? or \)
//
// It return a counter of matched non-wildcard characters, -1 if no match
func GlobMatch(str, pattern string) int {
	return globMatchInternal(str, pattern, 0)
}

// globMatchInternal does the actual work of GlobMatch() function
func globMatchInternal(str, pattern string, count int) int {
	for str != "" && pattern != "" {
		p := pattern[0]
		pattern = pattern[1:]

		switch p {
		case '*':
			for pattern != "" && pattern[0] == '*' {
				pattern = pattern[1:]
			}

			if pattern == "" {
				return count
			}

			for i := 0; i < len(str); i++ {
				c2 := globMatchInternal(str[i:], pattern, count)
				if c2 >= 0 {
					return c2
				}
			}

		case '?':
			str = str[1:]

		case '\\':
			if pattern == "" {
				return -1
			}
			p, pattern = pattern[0], pattern[1:]
			fallthrough

		default:
			if str[0] != p {
				return -1
			}
			str = str[1:]
			count++

		}
	}

	for pattern != "" && pattern[0] == '*' {
		pattern = pattern[1:]
	}

	if str == "" && pattern == "" {
		return count
	}

	return -1
}
