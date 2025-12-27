package main

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDedupeResolvedSpecsPreserveOrder(t *testing.T) {
	specs := []ResolvedSpec{
		{Source: "a", Origin: "seed"},
		{Source: "b", Origin: "seed"},
		{Source: "a", Origin: "discover"},
		{Source: "c", Origin: "discover"},
			{Source: "b", Origin: "spec-root"},
	}

	out := dedupeResolvedSpecsPreserveOrder(specs)
	assert.Equal(t, []ResolvedSpec{
		{Source: "a", Origin: "seed"},
		{Source: "b", Origin: "seed"},
		{Source: "c", Origin: "discover"},
	}, out)
}

func TestWriteResolvedSpecs_StableOnePerLine(t *testing.T) {
	buf := new(bytes.Buffer)
	writeResolvedSpecs(buf, []ResolvedSpec{{Source: "u1"}, {Source: "u2"}})
	assert.Equal(t, "Resolved specs\nu1\nu2\n", buf.String())
}
