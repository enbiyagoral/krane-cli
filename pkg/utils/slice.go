package utils

import (
	"regexp"
	"strings"
)

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

// FilterImages applies include/exclude patterns to image names.
// Include semantics: if includes non-empty, image must match at least one include (prefix or regex).
// Exclude semantics: if matches any exclude (prefix or regex), it is removed.
// Patterns that compile as regex are used as regex; otherwise prefix match is tried.
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

func (m matcher) match(s string) bool {
	if m.isRegex {
		return m.re.MatchString(s)
	}
	return strings.HasPrefix(s, m.prefix)
}

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
