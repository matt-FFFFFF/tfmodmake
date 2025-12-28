// Package hclgen provides helper functions for generating HCL files.
package hclgen

import (
	"os"
	"unicode"

	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"
)

func isSimpleIdentifier(s string) bool {
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
		if r != '_' && !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

// TokensForHeredoc returns tokens for a heredoc string.
func TokensForHeredoc(description string) hclwrite.Tokens {
	return hclwrite.Tokens{
		{
			Type:  hclsyntax.TokenOHeredoc,
			Bytes: []byte("<<DESCRIPTION\n"),
		},
		{
			Type:  hclsyntax.TokenStringLit,
			Bytes: []byte(description + "\n"),
		},
		{
			Type:  hclsyntax.TokenCHeredoc,
			Bytes: []byte("DESCRIPTION"),
		},
	}
}

// TokensForTraversal returns tokens for a dot-separated path of identifiers.
func TokensForTraversal(parts ...string) hclwrite.Tokens {
	var tokens hclwrite.Tokens
	for i, part := range parts {
		if i > 0 {
			tokens = append(tokens, &hclwrite.Token{
				Type:  hclsyntax.TokenDot,
				Bytes: []byte("."),
			})
		}
		tokens = append(tokens, &hclwrite.Token{
			Type:  hclsyntax.TokenIdent,
			Bytes: []byte(part),
		})
	}
	return tokens
}

// TokensForTraversalOrIndex returns tokens for accessing a path of keys, using
// dot traversal when possible and index syntax for non-identifier keys.
//
// Example:
//
//	azapi_resource.this.output["foo-bar"].baz
func TokensForTraversalOrIndex(parts ...string) hclwrite.Tokens {
	var tokens hclwrite.Tokens
	for i, part := range parts {
		if i == 0 {
			// Root must be an identifier.
			tokens = append(tokens, &hclwrite.Token{Type: hclsyntax.TokenIdent, Bytes: []byte(part)})
			continue
		}

		if isSimpleIdentifier(part) {
			tokens = append(tokens, &hclwrite.Token{Type: hclsyntax.TokenDot, Bytes: []byte(".")})
			tokens = append(tokens, &hclwrite.Token{Type: hclsyntax.TokenIdent, Bytes: []byte(part)})
			continue
		}

		tokens = append(tokens, &hclwrite.Token{Type: hclsyntax.TokenOBrack, Bytes: []byte("[")})
		tokens = append(tokens, hclwrite.TokensForValue(cty.StringVal(part))...)
		tokens = append(tokens, &hclwrite.Token{Type: hclsyntax.TokenCBrack, Bytes: []byte("]")})
	}
	return tokens
}

// NullEqualityTernary returns tokens for a ternary expression: condition == null ? null : trueExpr
func NullEqualityTernary(conditionExpr hclwrite.Tokens, trueExpr hclwrite.Tokens) hclwrite.Tokens {
	var t hclwrite.Tokens
	t = append(t, conditionExpr...)
	t = append(t, &hclwrite.Token{Type: hclsyntax.TokenEqualOp, Bytes: []byte("==")})
	t = append(t, hclwrite.TokensForIdentifier("null")...)
	t = append(t, &hclwrite.Token{Type: hclsyntax.TokenQuestion, Bytes: []byte("?")})
	t = append(t, hclwrite.TokensForIdentifier("null")...)
	t = append(t, &hclwrite.Token{Type: hclsyntax.TokenColon, Bytes: []byte(":")})
	t = append(t, trueExpr...)
	return t
}

// SetDescriptionAttribute sets the description attribute on a body using a heredoc.
func SetDescriptionAttribute(body *hclwrite.Body, description string) {
	body.SetAttributeRaw("description", TokensForHeredoc(description))
}

// TokensForMultilineStringList returns tokens for a list of strings formatted
// across multiple lines, e.g.:
// [
//
//	"a",
//	"b",
//
// ]
func TokensForMultilineStringList(values []string) hclwrite.Tokens {
	if len(values) == 0 {
		return hclwrite.TokensForValue(cty.ListValEmpty(cty.String))
	}

	tokens := hclwrite.Tokens{
		{Type: hclsyntax.TokenOBrack, Bytes: []byte("[")},
		{Type: hclsyntax.TokenNewline, Bytes: []byte("\n")},
	}

	for i, v := range values {
		tokens = append(tokens, hclwrite.TokensForValue(cty.StringVal(v))...)
		if i < len(values)-1 {
			tokens = append(tokens, &hclwrite.Token{Type: hclsyntax.TokenComma, Bytes: []byte(",")})
		}
		tokens = append(tokens, &hclwrite.Token{Type: hclsyntax.TokenNewline, Bytes: []byte("\n")})
	}

	tokens = append(tokens, &hclwrite.Token{Type: hclsyntax.TokenCBrack, Bytes: []byte("]")})
	return tokens
}

// WriteFile writes an HCL file to disk.
func WriteFile(path string, file *hclwrite.File) error {
	return os.WriteFile(path, file.Bytes(), 0o644)
}
