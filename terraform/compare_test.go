package terraform

import (
	"testing"

	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
)

func makeToken(typ hclsyntax.TokenType, b string) *hclwrite.Token {
	return &hclwrite.Token{Type: typ, Bytes: []byte(b)}
}

func TestNormalizeTokens(t *testing.T) {
	tests := []struct {
		name  string
		input hclwrite.Tokens
		want  int // expected number of tokens after normalization
	}{
		{
			name:  "empty tokens",
			input: hclwrite.Tokens{},
			want:  0,
		},
		{
			name: "strips newline tokens",
			input: hclwrite.Tokens{
				makeToken(hclsyntax.TokenIdent, "foo"),
				makeToken(hclsyntax.TokenNewline, "\n"),
				makeToken(hclsyntax.TokenIdent, "bar"),
			},
			want: 2,
		},
		{
			name: "strips comment tokens",
			input: hclwrite.Tokens{
				makeToken(hclsyntax.TokenIdent, "foo"),
				makeToken(hclsyntax.TokenComment, "# comment"),
				makeToken(hclsyntax.TokenIdent, "bar"),
			},
			want: 2,
		},
		{
			name: "strips whitespace-only tokens",
			input: hclwrite.Tokens{
				makeToken(hclsyntax.TokenIdent, "foo"),
				makeToken(hclsyntax.TokenIdent, "   "),
				makeToken(hclsyntax.TokenIdent, "bar"),
			},
			want: 2,
		},
		{
			name: "trims leading/trailing whitespace from tokens",
			input: hclwrite.Tokens{
				makeToken(hclsyntax.TokenIdent, "  foo  "),
			},
			want: 1,
		},
		{
			name: "preserves semantic tokens",
			input: hclwrite.Tokens{
				makeToken(hclsyntax.TokenIdent, "string"),
			},
			want: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeTokens(tt.input)
			if len(got) != tt.want {
				t.Errorf("NormalizeTokens() returned %d tokens, want %d", len(got), tt.want)
			}
		})
	}
}

func TestNormalizeTokens_trimmedContent(t *testing.T) {
	input := hclwrite.Tokens{
		makeToken(hclsyntax.TokenIdent, "  foo  "),
	}
	got := NormalizeTokens(input)
	if len(got) != 1 {
		t.Fatalf("expected 1 token, got %d", len(got))
	}
	if string(got[0].Bytes) != "foo" {
		t.Errorf("expected trimmed bytes %q, got %q", "foo", string(got[0].Bytes))
	}
}

func TestTokensEqual(t *testing.T) {
	tests := []struct {
		name string
		a    hclwrite.Tokens
		b    hclwrite.Tokens
		want bool
	}{
		{
			name: "identical tokens",
			a:    hclwrite.Tokens{makeToken(hclsyntax.TokenIdent, "string")},
			b:    hclwrite.Tokens{makeToken(hclsyntax.TokenIdent, "string")},
			want: true,
		},
		{
			name: "identical after whitespace normalization",
			a: hclwrite.Tokens{
				makeToken(hclsyntax.TokenIdent, "string"),
				makeToken(hclsyntax.TokenNewline, "\n"),
			},
			b: hclwrite.Tokens{
				makeToken(hclsyntax.TokenIdent, "string"),
			},
			want: true,
		},
		{
			name: "different content",
			a:    hclwrite.Tokens{makeToken(hclsyntax.TokenIdent, "string")},
			b:    hclwrite.Tokens{makeToken(hclsyntax.TokenIdent, "number")},
			want: false,
		},
		{
			name: "different types",
			a:    hclwrite.Tokens{makeToken(hclsyntax.TokenIdent, "foo")},
			b:    hclwrite.Tokens{makeToken(hclsyntax.TokenOQuote, "foo")},
			want: false,
		},
		{
			name: "different lengths",
			a: hclwrite.Tokens{
				makeToken(hclsyntax.TokenIdent, "a"),
				makeToken(hclsyntax.TokenIdent, "b"),
			},
			b:    hclwrite.Tokens{makeToken(hclsyntax.TokenIdent, "a")},
			want: false,
		},
		{
			name: "both empty",
			a:    hclwrite.Tokens{},
			b:    hclwrite.Tokens{},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := TokensEqual(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("TokensEqual() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCompareVariables(t *testing.T) {
	strTokens := hclwrite.Tokens{makeToken(hclsyntax.TokenIdent, "string")}
	numTokens := hclwrite.Tokens{makeToken(hclsyntax.TokenIdent, "number")}
	boolTokens := hclwrite.Tokens{makeToken(hclsyntax.TokenIdent, "bool")}

	tests := []struct {
		name         string
		onDisk       map[string]hclwrite.Tokens
		baseline     map[string]hclwrite.Tokens
		newGenerated map[string]hclwrite.Tokens
		want         map[string]CompareResult
	}{
		{
			name:         "identical variable",
			onDisk:       map[string]hclwrite.Tokens{"name": strTokens},
			baseline:     map[string]hclwrite.Tokens{"name": strTokens},
			newGenerated: map[string]hclwrite.Tokens{"name": strTokens},
			want:         map[string]CompareResult{"name": CompareIdentical},
		},
		{
			name:         "user modified variable",
			onDisk:       map[string]hclwrite.Tokens{"name": numTokens},
			baseline:     map[string]hclwrite.Tokens{"name": strTokens},
			newGenerated: map[string]hclwrite.Tokens{"name": strTokens},
			want:         map[string]CompareResult{"name": CompareModified},
		},
		{
			name:         "new variable in new spec",
			onDisk:       map[string]hclwrite.Tokens{},
			baseline:     map[string]hclwrite.Tokens{},
			newGenerated: map[string]hclwrite.Tokens{"tags": strTokens},
			want:         map[string]CompareResult{"tags": CompareNew},
		},
		{
			name:         "removed variable from new spec",
			onDisk:       map[string]hclwrite.Tokens{"old_field": strTokens},
			baseline:     map[string]hclwrite.Tokens{"old_field": strTokens},
			newGenerated: map[string]hclwrite.Tokens{},
			want:         map[string]CompareResult{"old_field": CompareRemoved},
		},
		{
			name:         "user deleted variable from disk",
			onDisk:       map[string]hclwrite.Tokens{},
			baseline:     map[string]hclwrite.Tokens{"name": strTokens},
			newGenerated: map[string]hclwrite.Tokens{"name": strTokens},
			want:         map[string]CompareResult{"name": CompareModified},
		},
		{
			name: "mixed scenario",
			onDisk: map[string]hclwrite.Tokens{
				"unchanged": strTokens,
				"modified":  numTokens,
				"removed":   boolTokens,
			},
			baseline: map[string]hclwrite.Tokens{
				"unchanged": strTokens,
				"modified":  strTokens,
				"removed":   boolTokens,
			},
			newGenerated: map[string]hclwrite.Tokens{
				"unchanged": strTokens,
				"modified":  strTokens,
				"added":     numTokens,
			},
			want: map[string]CompareResult{
				"unchanged": CompareIdentical,
				"modified":  CompareModified,
				"added":     CompareNew,
				"removed":   CompareRemoved,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CompareVariables(tt.onDisk, tt.baseline, tt.newGenerated)
			if len(got) != len(tt.want) {
				t.Fatalf("CompareVariables() returned %d results, want %d", len(got), len(tt.want))
			}
			for name, wantResult := range tt.want {
				gotResult, ok := got[name]
				if !ok {
					t.Errorf("CompareVariables() missing result for %q", name)
					continue
				}
				if gotResult != wantResult {
					t.Errorf("CompareVariables()[%q] = %d, want %d", name, gotResult, wantResult)
				}
			}
		})
	}
}

func TestCompareLocals(t *testing.T) {
	// CompareLocals delegates to CompareVariables, so just verify it works.
	tokA := hclwrite.Tokens{makeToken(hclsyntax.TokenIdent, "expr_a")}
	tokB := hclwrite.Tokens{makeToken(hclsyntax.TokenIdent, "expr_b")}

	onDisk := map[string]hclwrite.Tokens{"body": tokA}
	baseline := map[string]hclwrite.Tokens{"body": tokA}
	newGen := map[string]hclwrite.Tokens{"body": tokB}

	got := CompareLocals(onDisk, baseline, newGen)
	if got["body"] != CompareIdentical {
		t.Errorf("CompareLocals()[body] = %d, want %d (CompareIdentical)", got["body"], CompareIdentical)
	}
}
