package utils

import (
	"regexp"
	"strings"
)

// RemoveDuplicates removes duplicate strings from a slice while preserving order.
func RemoveDuplicates(slice []string) []string {
	keys := make(map[string]bool)
	var result []string

	for _, item := range slice {
		if !keys[item] {
			keys[item] = true
			result = append(result, item)
		}
	}
	return result
}

// FilterImages applies include/exclude patterns to filter image names.
func FilterImages(images []string, includes, excludes []string) ([]string, error) {
	var result []string
	incMatchers, err := compileMatchers(includes)
	if err != nil {
		return nil, err
	}
	excMatchers, err := compileMatchers(excludes)
	if err != nil {
		return nil, err
	}

	for _, img := range images {
		if len(incMatchers) > 0 {
			matched := false
			for _, m := range incMatchers {
				if m.match(img) {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
		}

		excluded := false
		for _, m := range excMatchers {
			if m.match(img) {
				excluded = true
				break
			}
		}
		if excluded {
			continue
		}
		result = append(result, img)
	}
	return result, nil
}

type matcher struct {
	isRegex bool
	prefix  string
	re      *regexp.Regexp
}

// match checks if the given string matches this matcher pattern.
func (m matcher) match(s string) bool {
	if m.isRegex {
		return m.re.MatchString(s)
	}
	return strings.HasPrefix(s, m.prefix)
}

// compileMatchers compiles string patterns into regex or prefix matchers.
func compileMatchers(patterns []string) ([]matcher, error) {
	var matchers []matcher
	for _, p := range patterns {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		// Try to compile as regex; if fails, treat as prefix
		re, err := regexp.Compile(p)
		if err != nil {
			matchers = append(matchers, matcher{isRegex: false, prefix: p})
			continue
		}
		matchers = append(matchers, matcher{isRegex: true, re: re})
	}
	return matchers, nil
}
