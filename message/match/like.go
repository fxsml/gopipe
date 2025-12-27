package match

import "strings"

// Like performs SQL LIKE pattern matching.
// % matches any sequence of characters.
// _ matches a single character.
func Like(pattern, value string) bool {
	return likeMatch(pattern, value)
}

func likeMatch(pattern, value string) bool {
	pi, vi := 0, 0
	pLen, vLen := len(pattern), len(value)
	starIdx, matchIdx := -1, 0

	for vi < vLen {
		if pi < pLen && (pattern[pi] == '_' || pattern[pi] == value[vi]) {
			pi++
			vi++
		} else if pi < pLen && pattern[pi] == '%' {
			starIdx = pi
			matchIdx = vi
			pi++
		} else if starIdx != -1 {
			pi = starIdx + 1
			matchIdx++
			vi = matchIdx
		} else {
			return false
		}
	}

	for pi < pLen && pattern[pi] == '%' {
		pi++
	}

	return pi == pLen
}

// LikeAny returns true if value matches any of the patterns.
func LikeAny(patterns []string, value string) bool {
	for _, p := range patterns {
		if Like(p, value) {
			return true
		}
	}
	return false
}

// getAttr retrieves a string attribute from a map.
func getAttr(attrs map[string]any, key string) string {
	if attrs == nil {
		return ""
	}
	v, ok := attrs[strings.ToLower(key)]
	if !ok {
		v, ok = attrs[key]
	}
	if !ok {
		return ""
	}
	s, _ := v.(string)
	return s
}
