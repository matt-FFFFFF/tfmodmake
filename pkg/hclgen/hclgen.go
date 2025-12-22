// Package hclgen provides helper functions for generating HCL files.
package hclgen

import (
	"os"

	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
)

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

// WriteFile writes an HCL file to disk.
func WriteFile(path string, file *hclwrite.File) error {
	return os.WriteFile(path, file.Bytes(), 0o644)
}
