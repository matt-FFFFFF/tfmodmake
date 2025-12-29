package terraform

import (
	"unicode"

	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"
)

func isHCLIdentifier(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		if i == 0 {
			if r != '_' && !unicode.IsLetter(r) {
				return false
			}
			continue
		}
		if r != '_' && r != '-' && !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

func tokensForObjectKey(key string) hclwrite.Tokens {
	if isHCLIdentifier(key) {
		return hclwrite.TokensForIdentifier(key)
	}
	return hclwrite.TokensForValue(cty.StringVal(key))
}
