package main

import (
	"testing"
)

func TestDeriveModuleName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple child resource",
			input:    "Microsoft.App/managedEnvironments/storages",
			expected: "storages",
		},
		{
			name:     "child resource with placeholder",
			input:    "Microsoft.App/managedEnvironments/storages/{storageName}",
			expected: "storages",
		},
		{
			name:     "KeyVault secrets",
			input:    "Microsoft.KeyVault/vaults/secrets",
			expected: "secrets",
		},
		{
			name:     "certificates",
			input:    "Microsoft.App/managedEnvironments/certificates",
			expected: "certificates",
		},
		{
			name:     "single segment",
			input:    "storages",
			expected: "storages",
		},
		{
			name:     "deeply nested",
			input:    "Microsoft.Foo/bars/bazs/quxs",
			expected: "quxs",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := deriveModuleName(tt.input)
			if result != tt.expected {
				t.Errorf("deriveModuleName(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}
