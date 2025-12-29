package naming

import (
	"strings"
	"unicode"
)

// ToSnakeCase converts mixedCase/PascalCase/other separator forms into snake_case.
func ToSnakeCase(input string) string {
	var sb strings.Builder
	runes := []rune(input)

	prevWasUnderscore := false
	wroteAny := false

	isAlnum := func(r rune) bool {
		return unicode.IsLetter(r) || unicode.IsDigit(r)
	}
	prevAlnum := func(i int) (rune, bool) {
		for j := i - 1; j >= 0; j-- {
			if isAlnum(runes[j]) {
				return runes[j], true
			}
		}
		return 0, false
	}
	nextAlnum := func(i int) (rune, bool) {
		for j := i + 1; j < len(runes); j++ {
			if isAlnum(runes[j]) {
				return runes[j], true
			}
		}
		return 0, false
	}

	for i, r := range runes {
		// Treat non-alphanumerics (e.g. '-', '.', spaces) as separators.
		if !isAlnum(r) {
			if wroteAny && !prevWasUnderscore {
				sb.WriteRune('_')
				prevWasUnderscore = true
			}
			continue
		}

		if unicode.IsUpper(r) {
			if p, ok := prevAlnum(i); ok {
				if (unicode.IsLower(p) || unicode.IsDigit(p)) && !prevWasUnderscore {
					sb.WriteRune('_')
				}
				if unicode.IsUpper(p) {
					// Split acronyms when the next alnum is lower (HTTPClient -> http_client)
					if n, ok := nextAlnum(i); ok && unicode.IsLower(n) {
						// Look ahead for a lower-case sequence length
						j := i + 1
						for j < len(runes) {
							if !isAlnum(runes[j]) {
								j++
								continue
							}
							if !unicode.IsLower(runes[j]) {
								break
							}
							j++
						}
						lowerLen := j - (i + 1)

						if lowerLen > 1 && !prevWasUnderscore {
							sb.WriteRune('_')
						}
						if lowerLen == 1 && n != 's' && !prevWasUnderscore {
							sb.WriteRune('_')
						}
					}
				}
			}
		}

		sb.WriteRune(unicode.ToLower(r))
		wroteAny = true
		prevWasUnderscore = false
	}

	out := strings.Trim(sb.String(), "_")
	if out == "" {
		return out
	}
	if len(out) > 0 && out[0] >= '0' && out[0] <= '9' {
		out = "field_" + out
	}
	return out
}
