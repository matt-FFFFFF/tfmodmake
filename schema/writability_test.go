package schema

import (
	"testing"

	"github.com/Azure/bicep-types/src/bicep-types-go/types"
	"github.com/stretchr/testify/assert"
)

func TestIsRequired(t *testing.T) {
	tests := []struct {
		name     string
		flags    types.TypePropertyFlags
		expected bool
	}{
		{"none", types.TypePropertyFlagsNone, false},
		{"required only", types.TypePropertyFlagsRequired, true},
		{"required and readonly", types.TypePropertyFlagsRequired | types.TypePropertyFlagsReadOnly, true},
		{"readonly only", types.TypePropertyFlagsReadOnly, false},
		{"writeonly only", types.TypePropertyFlagsWriteOnly, false},
		{"all flags", types.TypePropertyFlagsRequired | types.TypePropertyFlagsReadOnly | types.TypePropertyFlagsWriteOnly | types.TypePropertyFlagsDeployTimeConstant, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, IsRequired(tc.flags))
		})
	}
}

func TestIsReadOnly(t *testing.T) {
	tests := []struct {
		name     string
		flags    types.TypePropertyFlags
		expected bool
	}{
		{"none", types.TypePropertyFlagsNone, false},
		{"readonly only", types.TypePropertyFlagsReadOnly, true},
		{"required and readonly", types.TypePropertyFlagsRequired | types.TypePropertyFlagsReadOnly, true},
		{"required only", types.TypePropertyFlagsRequired, false},
		{"writeonly only", types.TypePropertyFlagsWriteOnly, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, IsReadOnly(tc.flags))
		})
	}
}

func TestIsWriteOnly(t *testing.T) {
	tests := []struct {
		name     string
		flags    types.TypePropertyFlags
		expected bool
	}{
		{"none", types.TypePropertyFlagsNone, false},
		{"writeonly only", types.TypePropertyFlagsWriteOnly, true},
		{"required and writeonly", types.TypePropertyFlagsRequired | types.TypePropertyFlagsWriteOnly, true},
		{"readonly only", types.TypePropertyFlagsReadOnly, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, IsWriteOnly(tc.flags))
		})
	}
}

func TestIsDeployTimeConstant(t *testing.T) {
	tests := []struct {
		name     string
		flags    types.TypePropertyFlags
		expected bool
	}{
		{"none", types.TypePropertyFlagsNone, false},
		{"deploytime only", types.TypePropertyFlagsDeployTimeConstant, true},
		{"required and deploytime", types.TypePropertyFlagsRequired | types.TypePropertyFlagsDeployTimeConstant, true},
		{"readonly only", types.TypePropertyFlagsReadOnly, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, IsDeployTimeConstant(tc.flags))
		})
	}
}

func TestIsWritable(t *testing.T) {
	tests := []struct {
		name     string
		flags    types.TypePropertyFlags
		expected bool
	}{
		{"none flags is writable", types.TypePropertyFlagsNone, true},
		{"required is writable", types.TypePropertyFlagsRequired, true},
		{"readonly is not writable", types.TypePropertyFlagsReadOnly, false},
		{"readonly and required is not writable", types.TypePropertyFlagsReadOnly | types.TypePropertyFlagsRequired, false},
		{"writeonly is writable", types.TypePropertyFlagsWriteOnly, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, IsWritable(tc.flags))
		})
	}
}

func TestIsComputed(t *testing.T) {
	tests := []struct {
		name     string
		flags    types.TypePropertyFlags
		expected bool
	}{
		{"none is not computed", types.TypePropertyFlagsNone, false},
		{"readonly is computed", types.TypePropertyFlagsReadOnly, true},
		{"required is not computed", types.TypePropertyFlagsRequired, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, IsComputed(tc.flags))
		})
	}
}
