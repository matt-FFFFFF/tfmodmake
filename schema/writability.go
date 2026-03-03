package schema

import (
	"github.com/Azure/bicep-types/src/bicep-types-go/types"
)

// IsRequired checks if the Required flag is set on a property's flags.
func IsRequired(flags types.TypePropertyFlags) bool {
	return flags&types.TypePropertyFlagsRequired != 0
}

// IsReadOnly checks if the ReadOnly flag is set on a property's flags.
func IsReadOnly(flags types.TypePropertyFlags) bool {
	return flags&types.TypePropertyFlagsReadOnly != 0
}

// IsWriteOnly checks if the WriteOnly flag is set on a property's flags.
func IsWriteOnly(flags types.TypePropertyFlags) bool {
	return flags&types.TypePropertyFlagsWriteOnly != 0
}

// IsDeployTimeConstant checks if the DeployTimeConstant flag is set on a property's flags.
func IsDeployTimeConstant(flags types.TypePropertyFlags) bool {
	return flags&types.TypePropertyFlagsDeployTimeConstant != 0
}

// IsWritable checks if a property is writable (not read-only).
// A property is writable if the ReadOnly flag is not set.
func IsWritable(flags types.TypePropertyFlags) bool {
	return !IsReadOnly(flags)
}

// IsComputed checks if a property is computed (read-only, set by the server).
func IsComputed(flags types.TypePropertyFlags) bool {
	return IsReadOnly(flags)
}
