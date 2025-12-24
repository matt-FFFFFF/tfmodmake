package terraform

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/matt-FFFFFF/tfmodmake/internal/hclgen"
	"github.com/matt-FFFFFF/tfmodmake/internal/openapi"
	"github.com/zclconf/go-cty/cty"
)

// generateValidations adds validation blocks to the variable body based on schema constraints.
// It generates null-safe validations for strings, arrays, numbers, and enums.
func generateValidations(varBody *hclwrite.Body, tfName string, propSchema *openapi3.Schema, isRequired bool) {
	if propSchema == nil {
		return
	}

	// Resolve schema references and allOf/oneOf/anyOf
	resolvedSchema := resolveSchemaForValidation(propSchema)

	// Generate enum validation
	generateEnumValidation(varBody, tfName, resolvedSchema, isRequired)

	// Generate string validations
	generateStringValidations(varBody, tfName, resolvedSchema, isRequired)

	// Generate array validations
	generateArrayValidations(varBody, tfName, resolvedSchema, isRequired)

	// Generate numeric validations
	generateNumericValidations(varBody, tfName, resolvedSchema, isRequired)
}

func generateNestedObjectValidations(varBody *hclwrite.Body, tfName string, objSchema *openapi3.Schema) error {
	if objSchema == nil || objSchema.Type == nil {
		return nil
	}
	if !slices.Contains(*objSchema.Type, "object") {
		return nil
	}

	// Nested validations are conservative, but allOf effective-shape errors (cycles/conflicts)
	// indicate structural schema problems and should fail generation loudly.
	effectiveProps, err := openapi.GetEffectiveProperties(objSchema)
	if err != nil {
		return fmt.Errorf("getting effective properties for nested validations (%s): %w", tfName, err)
	}
	if len(effectiveProps) == 0 {
		return nil
	}

	effectiveRequired, err := openapi.GetEffectiveRequired(objSchema)
	if err != nil {
		return fmt.Errorf("getting effective required for nested validations (%s): %w", tfName, err)
	}

	parentRef := hclgen.TokensForTraversal("var", tfName)

	type keyPair struct {
		original string
		snake    string
	}
	var keys []keyPair
	for k := range effectiveProps {
		snake := toSnakeCase(k)
		if snake == "" {
			continue
		}
		keys = append(keys, keyPair{original: k, snake: snake})
	}
	sort.Slice(keys, func(i, j int) bool {
		return keys[i].snake < keys[j].snake
	})

	for _, kp := range keys {
		prop := effectiveProps[kp.original]
		if prop == nil || prop.Value == nil {
			continue
		}
		if !isWritableProperty(prop.Value) {
			continue
		}

		childSchema := resolveSchemaForValidation(prop.Value)
		if childSchema == nil {
			continue
		}

		// Keep nested validations conservative: validate only scalar fields and arrays of scalars.
		if !isScalarOrScalarArraySchema(childSchema) {
			continue
		}

		childRef := hclgen.TokensForTraversal("var", tfName, kp.snake)
		displayName := fmt.Sprintf("%s.%s", tfName, kp.snake)
		childRequired := slices.Contains(effectiveRequired, kp.original)

		appendValidationsForExpr(varBody, displayName, parentRef, childRef, childSchema, childRequired)
	}

	return nil
}

func isScalarOrScalarArraySchema(schema *openapi3.Schema) bool {
	if schema == nil || schema.Type == nil {
		return false
	}
	if slices.Contains(*schema.Type, "string") || slices.Contains(*schema.Type, "integer") || slices.Contains(*schema.Type, "number") || slices.Contains(*schema.Type, "boolean") {
		return true
	}
	if !slices.Contains(*schema.Type, "array") {
		return false
	}
	if schema.Items == nil || schema.Items.Value == nil || schema.Items.Value.Type == nil {
		return false
	}
	itemTypes := *schema.Items.Value.Type
	return slices.Contains(itemTypes, "string") || slices.Contains(itemTypes, "integer") || slices.Contains(itemTypes, "number") || slices.Contains(itemTypes, "boolean")
}

func appendValidationsForExpr(varBody *hclwrite.Body, displayName string, parentRef, valueRef hclwrite.Tokens, schema *openapi3.Schema, isRequired bool) {
	// Enum
	if condition, ok := enumConditionTokens(valueRef, schema); ok {
		if !isRequired {
			condition = wrapWithNullGuard(valueRef, condition)
		}
		condition = wrapWithNullGuard(parentRef, condition)
		appendValidation(varBody, condition, fmt.Sprintf("%s must be one of: %s.", displayName, joinEnumValues(enumValuesForError(schema))))
	}

	// Strings
	if condition, ok := stringMinLengthConditionTokens(valueRef, schema); ok {
		if !isRequired {
			condition = wrapWithNullGuard(valueRef, condition)
		}
		condition = wrapWithNullGuard(parentRef, condition)
		appendValidation(varBody, condition, fmt.Sprintf("%s must have a minimum length of %d.", displayName, schema.MinLength))
	}
	if condition, ok := stringMaxLengthConditionTokens(valueRef, schema); ok {
		if !isRequired {
			condition = wrapWithNullGuard(valueRef, condition)
		}
		condition = wrapWithNullGuard(parentRef, condition)
		appendValidation(varBody, condition, fmt.Sprintf("%s must have a maximum length of %d.", displayName, *schema.MaxLength))
	}
	if condition, ok := stringFormatConditionTokens(valueRef, schema); ok {
		if !isRequired {
			condition = wrapWithNullGuard(valueRef, condition)
		}
		condition = wrapWithNullGuard(parentRef, condition)
		appendValidation(varBody, condition, fmt.Sprintf("%s must be a valid UUID.", displayName))
	}
	if condition, ok := stringPatternConditionTokens(valueRef, schema); ok {
		if !isRequired {
			condition = wrapWithNullGuard(valueRef, condition)
		}
		condition = wrapWithNullGuard(parentRef, condition)
		appendValidation(varBody, condition, fmt.Sprintf("%s must match the pattern: %s.", displayName, schema.Pattern))
	}

	// Arrays
	if condition, ok := arrayMinItemsConditionTokens(valueRef, schema); ok {
		if !isRequired {
			condition = wrapWithNullGuard(valueRef, condition)
		}
		condition = wrapWithNullGuard(parentRef, condition)
		appendValidation(varBody, condition, fmt.Sprintf("%s must have at least %d item(s).", displayName, schema.MinItems))
	}
	if condition, ok := arrayMaxItemsConditionTokens(valueRef, schema); ok {
		if !isRequired {
			condition = wrapWithNullGuard(valueRef, condition)
		}
		condition = wrapWithNullGuard(parentRef, condition)
		appendValidation(varBody, condition, fmt.Sprintf("%s must have at most %d item(s).", displayName, *schema.MaxItems))
	}
	if condition, ok := arrayUniqueItemsConditionTokens(valueRef, schema); ok {
		if !isRequired {
			condition = wrapWithNullGuard(valueRef, condition)
		}
		condition = wrapWithNullGuard(parentRef, condition)
		appendValidation(varBody, condition, fmt.Sprintf("%s must contain unique items.", displayName))
	}

	// Numbers
	if condition, msg, ok := numericMinimumConditionTokens(valueRef, schema, displayName); ok {
		if !isRequired {
			condition = wrapWithNullGuard(valueRef, condition)
		}
		condition = wrapWithNullGuard(parentRef, condition)
		appendValidation(varBody, condition, msg)
	}
	if condition, msg, ok := numericMaximumConditionTokens(valueRef, schema, displayName); ok {
		if !isRequired {
			condition = wrapWithNullGuard(valueRef, condition)
		}
		condition = wrapWithNullGuard(parentRef, condition)
		appendValidation(varBody, condition, msg)
	}
	if condition, ok := numericMultipleOfConditionTokens(valueRef, schema); ok {
		if !isRequired {
			condition = wrapWithNullGuard(valueRef, condition)
		}
		condition = wrapWithNullGuard(parentRef, condition)
		appendValidation(varBody, condition, fmt.Sprintf("%s must be a multiple of %v.", displayName, *schema.MultipleOf))
	}
}

func appendValidation(varBody *hclwrite.Body, condition hclwrite.Tokens, errorMessage string) {
	validation := varBody.AppendNewBlock("validation", nil)
	validationBody := validation.Body()
	validationBody.SetAttributeRaw("condition", condition)
	validationBody.SetAttributeValue("error_message", cty.StringVal(errorMessage))
}

func wrapWithNullGuard(nullRef, inner hclwrite.Tokens) hclwrite.Tokens {
	if len(nullRef) == 0 {
		return inner
	}
	var out hclwrite.Tokens
	out = append(out, nullRef...)
	out = append(out, &hclwrite.Token{Type: hclsyntax.TokenEqualOp, Bytes: []byte(" == ")})
	out = append(out, hclwrite.TokensForIdentifier("null")...)
	out = append(out, &hclwrite.Token{Type: hclsyntax.TokenOr, Bytes: []byte(" || ")})
	out = append(out, inner...)
	return out
}

func enumValuesForError(schema *openapi3.Schema) []string {
	if schema == nil {
		return nil
	}
	values, ok := enumValues(schema)
	if !ok {
		return nil
	}
	return values
}

func enumValues(schema *openapi3.Schema) ([]string, bool) {
	if schema == nil {
		return nil, false
	}

	var enumValues []any
	if len(schema.Enum) > 0 {
		enumValues = schema.Enum
	}

	if len(enumValues) == 0 && schema.Extensions != nil {
		if xMsEnum, ok := schema.Extensions["x-ms-enum"]; ok {
			if enumMap, ok := xMsEnum.(map[string]any); ok {
				if values, ok := enumMap["values"]; ok {
					if valuesSlice, ok := values.([]any); ok {
						for _, v := range valuesSlice {
							if valueMap, ok := v.(map[string]any); ok {
								if value, ok := valueMap["value"]; ok {
									enumValues = append(enumValues, value)
								}
							}
						}
					}
				}
			}
		}
	}

	if len(enumValues) == 0 {
		return nil, false
	}

	var raw []string
	for _, v := range enumValues {
		raw = append(raw, fmt.Sprintf("%v", v))
	}
	sort.Strings(raw)
	return raw, true
}

func enumConditionTokens(valueRef hclwrite.Tokens, schema *openapi3.Schema) (hclwrite.Tokens, bool) {
	values, ok := enumValues(schema)
	if !ok {
		return nil, false
	}
	var enumTokens []hclwrite.Tokens
	for _, v := range values {
		enumTokens = append(enumTokens, hclwrite.TokensForValue(cty.StringVal(v)))
	}
	enumList := hclwrite.TokensForTuple(enumTokens)
	containsCall := hclwrite.TokensForFunctionCall("contains", enumList, valueRef)
	return containsCall, true
}

func stringMinLengthConditionTokens(valueRef hclwrite.Tokens, schema *openapi3.Schema) (hclwrite.Tokens, bool) {
	if schema == nil || schema.Type == nil || !slices.Contains(*schema.Type, "string") {
		return nil, false
	}
	if schema.MinLength == 0 {
		return nil, false
	}
	lengthCall := hclwrite.TokensForFunctionCall("length", valueRef)
	var condition hclwrite.Tokens
	condition = append(condition, lengthCall...)
	condition = append(condition, &hclwrite.Token{Type: hclsyntax.TokenGreaterThanEq, Bytes: []byte(" >= ")})
	condition = append(condition, hclwrite.TokensForValue(cty.NumberIntVal(int64(schema.MinLength)))...)
	return condition, true
}

func stringMaxLengthConditionTokens(valueRef hclwrite.Tokens, schema *openapi3.Schema) (hclwrite.Tokens, bool) {
	if schema == nil || schema.Type == nil || !slices.Contains(*schema.Type, "string") {
		return nil, false
	}
	if schema.MaxLength == nil {
		return nil, false
	}
	lengthCall := hclwrite.TokensForFunctionCall("length", valueRef)
	var condition hclwrite.Tokens
	condition = append(condition, lengthCall...)
	condition = append(condition, &hclwrite.Token{Type: hclsyntax.TokenLessThanEq, Bytes: []byte(" <= ")})
	condition = append(condition, hclwrite.TokensForValue(cty.NumberUIntVal(*schema.MaxLength))...)
	return condition, true
}

func stringFormatConditionTokens(valueRef hclwrite.Tokens, schema *openapi3.Schema) (hclwrite.Tokens, bool) {
	if schema == nil || schema.Type == nil || !slices.Contains(*schema.Type, "string") {
		return nil, false
	}
	if schema.Format != "uuid" {
		return nil, false
	}
	regexPattern := "^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$"
	regexCall := hclwrite.TokensForFunctionCall("can",
		hclwrite.TokensForFunctionCall("regex",
			hclwrite.TokensForValue(cty.StringVal(regexPattern)),
			valueRef,
		),
	)
	return regexCall, true
}

func stringPatternConditionTokens(valueRef hclwrite.Tokens, schema *openapi3.Schema) (hclwrite.Tokens, bool) {
	if schema == nil || schema.Type == nil || !slices.Contains(*schema.Type, "string") {
		return nil, false
	}
	if schema.Pattern == "" {
		return nil, false
	}
	regexCall := hclwrite.TokensForFunctionCall("can",
		hclwrite.TokensForFunctionCall("regex",
			hclwrite.TokensForValue(cty.StringVal(schema.Pattern)),
			valueRef,
		),
	)
	return regexCall, true
}

func arrayMinItemsConditionTokens(valueRef hclwrite.Tokens, schema *openapi3.Schema) (hclwrite.Tokens, bool) {
	if schema == nil || schema.Type == nil || !slices.Contains(*schema.Type, "array") {
		return nil, false
	}
	if schema.MinItems == 0 {
		return nil, false
	}
	lengthCall := hclwrite.TokensForFunctionCall("length", valueRef)
	var condition hclwrite.Tokens
	condition = append(condition, lengthCall...)
	condition = append(condition, &hclwrite.Token{Type: hclsyntax.TokenGreaterThanEq, Bytes: []byte(" >= ")})
	condition = append(condition, hclwrite.TokensForValue(cty.NumberUIntVal(schema.MinItems))...)
	return condition, true
}

func arrayMaxItemsConditionTokens(valueRef hclwrite.Tokens, schema *openapi3.Schema) (hclwrite.Tokens, bool) {
	if schema == nil || schema.Type == nil || !slices.Contains(*schema.Type, "array") {
		return nil, false
	}
	if schema.MaxItems == nil {
		return nil, false
	}
	lengthCall := hclwrite.TokensForFunctionCall("length", valueRef)
	var condition hclwrite.Tokens
	condition = append(condition, lengthCall...)
	condition = append(condition, &hclwrite.Token{Type: hclsyntax.TokenLessThanEq, Bytes: []byte(" <= ")})
	condition = append(condition, hclwrite.TokensForValue(cty.NumberUIntVal(*schema.MaxItems))...)
	return condition, true
}

func arrayUniqueItemsConditionTokens(valueRef hclwrite.Tokens, schema *openapi3.Schema) (hclwrite.Tokens, bool) {
	if schema == nil || schema.Type == nil || !slices.Contains(*schema.Type, "array") {
		return nil, false
	}
	if !schema.UniqueItems {
		return nil, false
	}
	lengthCall := hclwrite.TokensForFunctionCall("length", valueRef)
	distinctCall := hclwrite.TokensForFunctionCall("distinct", valueRef)
	lengthDistinctCall := hclwrite.TokensForFunctionCall("length", distinctCall)
	var condition hclwrite.Tokens
	condition = append(condition, lengthDistinctCall...)
	condition = append(condition, &hclwrite.Token{Type: hclsyntax.TokenEqualOp, Bytes: []byte(" == ")})
	condition = append(condition, lengthCall...)
	return condition, true
}

func numericMinimumConditionTokens(valueRef hclwrite.Tokens, schema *openapi3.Schema, displayName string) (hclwrite.Tokens, string, bool) {
	if schema == nil || schema.Type == nil {
		return nil, "", false
	}
	if !slices.Contains(*schema.Type, "integer") && !slices.Contains(*schema.Type, "number") {
		return nil, "", false
	}
	if schema.Min == nil {
		return nil, "", false
	}
	var condition hclwrite.Tokens
	condition = append(condition, valueRef...)
	if schema.ExclusiveMin {
		condition = append(condition, &hclwrite.Token{Type: hclsyntax.TokenGreaterThan, Bytes: []byte(" > ")})
	} else {
		condition = append(condition, &hclwrite.Token{Type: hclsyntax.TokenGreaterThanEq, Bytes: []byte(" >= ")})
	}
	condition = append(condition, hclwrite.TokensForValue(cty.NumberFloatVal(*schema.Min))...)

	if schema.ExclusiveMin {
		return condition, fmt.Sprintf("%s must be greater than %v.", displayName, *schema.Min), true
	}
	return condition, fmt.Sprintf("%s must be greater than or equal to %v.", displayName, *schema.Min), true
}

func numericMaximumConditionTokens(valueRef hclwrite.Tokens, schema *openapi3.Schema, displayName string) (hclwrite.Tokens, string, bool) {
	if schema == nil || schema.Type == nil {
		return nil, "", false
	}
	if !slices.Contains(*schema.Type, "integer") && !slices.Contains(*schema.Type, "number") {
		return nil, "", false
	}
	if schema.Max == nil {
		return nil, "", false
	}
	var condition hclwrite.Tokens
	condition = append(condition, valueRef...)
	if schema.ExclusiveMax {
		condition = append(condition, &hclwrite.Token{Type: hclsyntax.TokenLessThan, Bytes: []byte(" < ")})
	} else {
		condition = append(condition, &hclwrite.Token{Type: hclsyntax.TokenLessThanEq, Bytes: []byte(" <= ")})
	}
	condition = append(condition, hclwrite.TokensForValue(cty.NumberFloatVal(*schema.Max))...)

	if schema.ExclusiveMax {
		return condition, fmt.Sprintf("%s must be less than %v.", displayName, *schema.Max), true
	}
	return condition, fmt.Sprintf("%s must be less than or equal to %v.", displayName, *schema.Max), true
}

func numericMultipleOfConditionTokens(valueRef hclwrite.Tokens, schema *openapi3.Schema) (hclwrite.Tokens, bool) {
	if schema == nil || schema.Type == nil {
		return nil, false
	}
	if !slices.Contains(*schema.Type, "integer") && !slices.Contains(*schema.Type, "number") {
		return nil, false
	}
	if schema.MultipleOf == nil {
		return nil, false
	}
	modCall := hclwrite.TokensForFunctionCall("abs",
		hclwrite.TokensForFunctionCall("mod",
			valueRef,
			hclwrite.TokensForValue(cty.NumberFloatVal(*schema.MultipleOf)),
		),
	)
	var condition hclwrite.Tokens
	condition = append(condition, modCall...)
	condition = append(condition, &hclwrite.Token{Type: hclsyntax.TokenLessThan, Bytes: []byte(" < ")})
	condition = append(condition, hclwrite.TokensForValue(cty.NumberFloatVal(0.000001))...)
	return condition, true
}

// resolveSchemaForValidation resolves $ref, allOf, oneOf, anyOf to get effective schema for validation.
func resolveSchemaForValidation(schema *openapi3.Schema) *openapi3.Schema {
	if schema == nil {
		return nil
	}

	// Start with the base schema
	resolved := schema

	// Handle allOf - merge all schemas.
	// OpenAPI allOf semantics are effectively an intersection: the result must satisfy all subschemas.
	// For numeric/length/item bounds, we therefore prefer the most restrictive constraint.
	if len(schema.AllOf) > 0 {
		merged := &openapi3.Schema{}
		// Copy from base schema
		if schema.Type != nil {
			merged.Type = schema.Type
		}
		if len(schema.Enum) > 0 {
			merged.Enum = schema.Enum
		}
		merged.MinLength = schema.MinLength
		merged.MaxLength = schema.MaxLength
		merged.Pattern = schema.Pattern
		merged.Min = schema.Min
		merged.Max = schema.Max
		merged.ExclusiveMin = schema.ExclusiveMin
		merged.ExclusiveMax = schema.ExclusiveMax
		merged.MultipleOf = schema.MultipleOf
		merged.MinItems = schema.MinItems
		merged.MaxItems = schema.MaxItems
		merged.UniqueItems = schema.UniqueItems
		merged.Format = schema.Format
		if schema.Extensions != nil {
			merged.Extensions = make(map[string]any)
			for k, v := range schema.Extensions {
				merged.Extensions[k] = v
			}
		}

		// Helper: intersect enum sets represented as []any.
		intersectEnum := func(a, b []any) []any {
			if len(a) == 0 {
				return b
			}
			if len(b) == 0 {
				return a
			}
			setA := make(map[string]struct{}, len(a))
			for _, v := range a {
				setA[fmt.Sprintf("%v", v)] = struct{}{}
			}
			var out []any
			for _, v := range b {
				s := fmt.Sprintf("%v", v)
				if _, ok := setA[s]; ok {
					out = append(out, s)
				}
			}
			return out
		}

		// Merge from each allOf schema
		for _, schemaRef := range schema.AllOf {
			if schemaRef.Value != nil {
				s := schemaRef.Value
				if s.Type != nil && merged.Type == nil {
					merged.Type = s.Type
				}

				if len(s.Enum) > 0 {
					merged.Enum = intersectEnum(merged.Enum, s.Enum)
				}

				if s.MinLength != 0 && s.MinLength > merged.MinLength {
					merged.MinLength = s.MinLength
				}
				if s.MaxLength != nil {
					if merged.MaxLength == nil || *s.MaxLength < *merged.MaxLength {
						merged.MaxLength = s.MaxLength
					}
				}

				if s.MinItems != 0 && s.MinItems > merged.MinItems {
					merged.MinItems = s.MinItems
				}
				if s.MaxItems != nil {
					if merged.MaxItems == nil || *s.MaxItems < *merged.MaxItems {
						merged.MaxItems = s.MaxItems
					}
				}
				if s.UniqueItems {
					merged.UniqueItems = true
				}

				if s.Min != nil {
					if merged.Min == nil || *s.Min > *merged.Min {
						merged.Min = s.Min
						merged.ExclusiveMin = s.ExclusiveMin
					} else if merged.Min != nil && *s.Min == *merged.Min {
						// If the same bound appears multiple times, exclusive is more restrictive.
						merged.ExclusiveMin = merged.ExclusiveMin || s.ExclusiveMin
					}
				}
				if s.Max != nil {
					if merged.Max == nil || *s.Max < *merged.Max {
						merged.Max = s.Max
						merged.ExclusiveMax = s.ExclusiveMax
					} else if merged.Max != nil && *s.Max == *merged.Max {
						merged.ExclusiveMax = merged.ExclusiveMax || s.ExclusiveMax
					}
				}

				if s.MultipleOf != nil && merged.MultipleOf == nil {
					merged.MultipleOf = s.MultipleOf
				}
				if s.Format != "" && merged.Format == "" {
					merged.Format = s.Format
				}
				if s.Pattern != "" && merged.Pattern == "" {
					merged.Pattern = s.Pattern
				}
				// Merge extensions (e.g., x-ms-enum)
				if s.Extensions != nil {
					if merged.Extensions == nil {
						merged.Extensions = make(map[string]any)
					}
					for k, v := range s.Extensions {
						if _, exists := merged.Extensions[k]; !exists {
							merged.Extensions[k] = v
						}
					}
				}
			}
		}
		resolved = merged
	}

	return resolved
}

// generateEnumValidation generates validation for enum values.
func generateEnumValidation(varBody *hclwrite.Body, tfName string, schema *openapi3.Schema, isRequired bool) {
	if schema == nil {
		return
	}

	varRef := hclgen.TokensForTraversal("var", tfName)
	condition, ok := enumConditionTokens(varRef, schema)
	if !ok {
		return
	}
	if !isRequired {
		condition = wrapWithNullGuard(varRef, condition)
	}
	appendValidation(varBody, condition, fmt.Sprintf("%s must be one of: %s.", tfName, joinEnumValues(enumValuesForError(schema))))
}

// joinEnumValues joins enum values for error messages, limiting to a reasonable length.
func joinEnumValues(values []string) string {
	const maxLength = 200
	const maxCount = 10

	if len(values) == 0 {
		return "[]"
	}

	quoted := make([]string, 0, len(values))
	for _, v := range values {
		quoted = append(quoted, fmt.Sprintf("%q", v))
	}

	if len(values) <= maxCount {
		joined := fmt.Sprintf("[%s]", strings.Join(quoted, ", "))
		if len(joined) <= maxLength {
			return joined
		}
	}

	// Too many or too long, show first few and count
	out := "["
	count := 0
	for i := 0; i < len(quoted) && i < maxCount; i++ {
		part := quoted[i]
		if count > 0 {
			part = ", " + part
		}
		// +1 for the closing bracket
		if len(out)+len(part)+1 > maxLength {
			break
		}
		out += part
		count++
	}
	out += "]"
	if count < len(values) {
		return fmt.Sprintf("%s (and %d more)", out, len(values)-count)
	}
	return out
}

// generateStringValidations generates validation for string constraints.
func generateStringValidations(varBody *hclwrite.Body, tfName string, schema *openapi3.Schema, isRequired bool) {
	if schema == nil || schema.Type == nil {
		return
	}

	if !slices.Contains(*schema.Type, "string") {
		return
	}

	varRef := hclgen.TokensForTraversal("var", tfName)

	if condition, ok := stringMinLengthConditionTokens(varRef, schema); ok {
		if !isRequired {
			condition = wrapWithNullGuard(varRef, condition)
		}
		appendValidation(varBody, condition, fmt.Sprintf("%s must have a minimum length of %d.", tfName, schema.MinLength))
	}

	if condition, ok := stringMaxLengthConditionTokens(varRef, schema); ok {
		if !isRequired {
			condition = wrapWithNullGuard(varRef, condition)
		}
		appendValidation(varBody, condition, fmt.Sprintf("%s must have a maximum length of %d.", tfName, *schema.MaxLength))
	}

	if condition, ok := stringFormatConditionTokens(varRef, schema); ok {
		if !isRequired {
			condition = wrapWithNullGuard(varRef, condition)
		}
		appendValidation(varBody, condition, fmt.Sprintf("%s must be a valid UUID.", tfName))
	}

	if condition, ok := stringPatternConditionTokens(varRef, schema); ok {
		if !isRequired {
			condition = wrapWithNullGuard(varRef, condition)
		}
		appendValidation(varBody, condition, fmt.Sprintf("%s must match the pattern: %s.", tfName, schema.Pattern))
	}
}

// generateArrayValidations generates validation for array/list constraints.
func generateArrayValidations(varBody *hclwrite.Body, tfName string, schema *openapi3.Schema, isRequired bool) {
	if schema == nil || schema.Type == nil {
		return
	}

	if !slices.Contains(*schema.Type, "array") {
		return
	}

	varRef := hclgen.TokensForTraversal("var", tfName)

	if condition, ok := arrayMinItemsConditionTokens(varRef, schema); ok {
		if !isRequired {
			condition = wrapWithNullGuard(varRef, condition)
		}
		appendValidation(varBody, condition, fmt.Sprintf("%s must have at least %d item(s).", tfName, schema.MinItems))
	}

	if condition, ok := arrayMaxItemsConditionTokens(varRef, schema); ok {
		if !isRequired {
			condition = wrapWithNullGuard(varRef, condition)
		}
		appendValidation(varBody, condition, fmt.Sprintf("%s must have at most %d item(s).", tfName, *schema.MaxItems))
	}

	if condition, ok := arrayUniqueItemsConditionTokens(varRef, schema); ok {
		if !isRequired {
			condition = wrapWithNullGuard(varRef, condition)
		}
		appendValidation(varBody, condition, fmt.Sprintf("%s must contain unique items.", tfName))
	}
}

// generateNumericValidations generates validation for numeric constraints.
func generateNumericValidations(varBody *hclwrite.Body, tfName string, schema *openapi3.Schema, isRequired bool) {
	if schema == nil || schema.Type == nil {
		return
	}

	if !slices.Contains(*schema.Type, "integer") && !slices.Contains(*schema.Type, "number") {
		return
	}

	varRef := hclgen.TokensForTraversal("var", tfName)

	if condition, msg, ok := numericMinimumConditionTokens(varRef, schema, tfName); ok {
		if !isRequired {
			condition = wrapWithNullGuard(varRef, condition)
		}
		appendValidation(varBody, condition, msg)
	}

	if condition, msg, ok := numericMaximumConditionTokens(varRef, schema, tfName); ok {
		if !isRequired {
			condition = wrapWithNullGuard(varRef, condition)
		}
		appendValidation(varBody, condition, msg)
	}

	if condition, ok := numericMultipleOfConditionTokens(varRef, schema); ok {
		if !isRequired {
			condition = wrapWithNullGuard(varRef, condition)
		}
		appendValidation(varBody, condition, fmt.Sprintf("%s must be a multiple of %v.", tfName, *schema.MultipleOf))
	}
}
