package schema

import (
	"testing"

	"github.com/Azure/bicep-types/src/bicep-types-go/index"
	"github.com/Azure/bicep-types/src/bicep-types-go/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDiscoverChildren(t *testing.T) {
	t.Run("finds direct children", func(t *testing.T) {
		idx := index.NewTypeIndex()
		// Parent
		idx.AddResource("Microsoft.Test/resources", "2023-01-01", types.CrossFileTypeReference{Ref: 0, RelativePath: "types.json"})
		// Direct child
		idx.AddResource("Microsoft.Test/resources/child1", "2023-01-01", types.CrossFileTypeReference{Ref: 1, RelativePath: "types.json"})
		// Another direct child
		idx.AddResource("Microsoft.Test/resources/child2", "2023-01-01", types.CrossFileTypeReference{Ref: 2, RelativePath: "types.json"})
		// Grandchild
		idx.AddResource("Microsoft.Test/resources/child1/grandchild", "2023-01-01", types.CrossFileTypeReference{Ref: 3, RelativePath: "types.json"})
		// Unrelated resource
		idx.AddResource("Microsoft.Other/stuff", "2023-01-01", types.CrossFileTypeReference{Ref: 4, RelativePath: "types.json"})

		children := DiscoverChildren(idx, "Microsoft.Test/resources", 1)

		// depth=1 should return only direct children, not grandchild
		assert.Len(t, children, 2)

		childTypes := make(map[string]bool)
		for _, c := range children {
			childTypes[c.ResourceType] = true
		}
		assert.True(t, childTypes["Microsoft.Test/resources/child1"])
		assert.True(t, childTypes["Microsoft.Test/resources/child2"])
		assert.False(t, childTypes["Microsoft.Test/resources/child1/grandchild"])
	})

	t.Run("finds all descendants with unlimited depth", func(t *testing.T) {
		idx := index.NewTypeIndex()
		idx.AddResource("Microsoft.Test/resources", "2023-01-01", types.CrossFileTypeReference{Ref: 0, RelativePath: "types.json"})
		idx.AddResource("Microsoft.Test/resources/child1", "2023-01-01", types.CrossFileTypeReference{Ref: 1, RelativePath: "types.json"})
		idx.AddResource("Microsoft.Test/resources/child1/grandchild", "2023-01-01", types.CrossFileTypeReference{Ref: 2, RelativePath: "types.json"})

		children := DiscoverChildren(idx, "Microsoft.Test/resources", 0)

		assert.Len(t, children, 2)

		childTypes := make(map[string]bool)
		for _, c := range children {
			childTypes[c.ResourceType] = true
		}
		assert.True(t, childTypes["Microsoft.Test/resources/child1"])
		assert.True(t, childTypes["Microsoft.Test/resources/child1/grandchild"])
	})

	t.Run("returns empty for resource with no children", func(t *testing.T) {
		idx := index.NewTypeIndex()
		idx.AddResource("Microsoft.Test/resources", "2023-01-01", types.CrossFileTypeReference{Ref: 0, RelativePath: "types.json"})
		idx.AddResource("Microsoft.Other/stuff", "2023-01-01", types.CrossFileTypeReference{Ref: 1, RelativePath: "types.json"})

		children := DiscoverChildren(idx, "Microsoft.Test/resources", 0)
		assert.Empty(t, children)
	})

	t.Run("child has multiple API versions", func(t *testing.T) {
		idx := index.NewTypeIndex()
		idx.AddResource("Microsoft.Test/resources", "2023-01-01", types.CrossFileTypeReference{Ref: 0, RelativePath: "types.json"})
		idx.AddResource("Microsoft.Test/resources/child1", "2023-01-01", types.CrossFileTypeReference{Ref: 1, RelativePath: "types.json"})
		idx.AddResource("Microsoft.Test/resources/child1", "2024-01-01", types.CrossFileTypeReference{Ref: 2, RelativePath: "types2.json"})

		children := DiscoverChildren(idx, "Microsoft.Test/resources", 1)

		require.Len(t, children, 1)
		assert.Equal(t, "Microsoft.Test/resources/child1", children[0].ResourceType)
		assert.Len(t, children[0].APIVersions, 2)

		versions := make(map[string]bool)
		for _, v := range children[0].APIVersions {
			versions[v] = true
		}
		assert.True(t, versions["2023-01-01"])
		assert.True(t, versions["2024-01-01"])
	})

	t.Run("depth 2 includes grandchildren but not great-grandchildren", func(t *testing.T) {
		idx := index.NewTypeIndex()
		idx.AddResource("Microsoft.Test/resources", "2023-01-01", types.CrossFileTypeReference{Ref: 0, RelativePath: "types.json"})
		idx.AddResource("Microsoft.Test/resources/child", "2023-01-01", types.CrossFileTypeReference{Ref: 1, RelativePath: "types.json"})
		idx.AddResource("Microsoft.Test/resources/child/grandchild", "2023-01-01", types.CrossFileTypeReference{Ref: 2, RelativePath: "types.json"})
		idx.AddResource("Microsoft.Test/resources/child/grandchild/greatgrandchild", "2023-01-01", types.CrossFileTypeReference{Ref: 3, RelativePath: "types.json"})

		children := DiscoverChildren(idx, "Microsoft.Test/resources", 2)

		childTypes := make(map[string]bool)
		for _, c := range children {
			childTypes[c.ResourceType] = true
		}
		assert.True(t, childTypes["Microsoft.Test/resources/child"])
		assert.True(t, childTypes["Microsoft.Test/resources/child/grandchild"])
		assert.False(t, childTypes["Microsoft.Test/resources/child/grandchild/greatgrandchild"])
		assert.Len(t, children, 2)
	})
}
