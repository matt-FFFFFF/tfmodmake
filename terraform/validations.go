package terraform

import (
	"fmt"
	"sort"
	"strings"

	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/matt-FFFFFF/tfmodmake/hclgen"
	"github.com/matt-FFFFFF/tfmodmake/naming"
	"github.com/matt-FFFFFF/tfmodmake/schema"
	"github.com/zclconf/go-cty/cty"
)

// generateValidations adds validation blocks to the variable body based on schema constraints.
// It generates null-safe validations for strings, arrays, numbers, and enums.
func generateValidations(varBody *hclwrite.Body, tfName string, prop *schema.Property, isRequired bool) {
	if prop == nil {
		return
	}

	// Generate enum validation
	generateEnumValidation(varBody, tfName, prop, isRequired)

	// Generate string validations
	generateStringValidations(varBody, tfName, prop, isRequired)

	// Generate array validations
	generateArrayValidations(varBody, tfName, prop, isRequired)

	// Generate numeric validations
	generateNumericValidations(varBody, tfName, prop, isRequired)
}

func generateNestedObjectValidations(varBody *hclwrite.Body, tfName string, prop *schema.Property) error {
	if prop == nil {
		return nil
	}
	if prop.Type != schema.TypeObject {
		return nil
	}

	if len(prop.Children) == 0 {
		return nil
	}

	parentRef := hclgen.TokensForTraversal("var", tfName)

	type keyPair struct {
		original string
		snake    string
	}
	var keys []keyPair
	for k := range prop.Children {
		snake := naming.ToSnakeCase(k)
		if snake == "" {
			continue
		}
		keys = append(keys, keyPair{original: k, snake: snake})
	}
	sort.Slice(keys, func(i, j int) bool {
		return keys[i].snake < keys[j].snake
	})

	for _, kp := range keys {
		child := prop.Children[kp.original]
		if child == nil {
			continue
		}
		if !isWritableProperty(child) {
			continue
		}

		// Keep nested validations conservative: validate only scalar fields and arrays of scalars.
		if !isScalarOrScalarArrayProp(child) {
			continue
		}

		childRef := hclgen.TokensForTraversal("var", tfName, kp.snake)
		displayName := fmt.Sprintf("%s.%s", tfName, kp.snake)
		childRequired := child.Required

		appendValidationsForExpr(varBody, displayName, parentRef, childRef, child, childRequired)
	}

	return nil
}

func isScalarOrScalarArrayProp(prop *schema.Property) bool {
	if prop == nil {
		return false
	}
	if prop.IsScalar() {
		return true
	}
	if prop.Type != schema.TypeArray {
		return false
	}
	if prop.ItemType == nil {
		return false
	}
	return prop.ItemType.IsScalar()
}

func appendValidationsForExpr(varBody *hclwrite.Body, displayName string, parentRef, valueRef hclwrite.Tokens, prop *schema.Property, isRequired bool) {
	// Enum
	if condition, ok := enumConditionTokens(valueRef, prop); ok {
		if !isRequired {
			condition = wrapWithNullGuard(valueRef, condition)
		}
		condition = wrapWithNullGuard(parentRef, condition)
		appendValidation(varBody, condition, fmt.Sprintf("%s must be one of: %s.", displayName, joinEnumValues(enumValuesForError(prop))))
	}

	// Strings
	if condition, ok := stringMinLengthConditionTokens(valueRef, prop); ok {
		if !isRequired {
			condition = wrapWithNullGuard(valueRef, condition)
		}
		condition = wrapWithNullGuard(parentRef, condition)
		appendValidation(varBody, condition, fmt.Sprintf("%s must have a minimum length of %d.", displayName, *prop.Constraints.MinLength))
	}
	if condition, ok := stringMaxLengthConditionTokens(valueRef, prop); ok {
		if !isRequired {
			condition = wrapWithNullGuard(valueRef, condition)
		}
		condition = wrapWithNullGuard(parentRef, condition)
		appendValidation(varBody, condition, fmt.Sprintf("%s must have a maximum length of %d.", displayName, *prop.Constraints.MaxLength))
	}
	if condition, ok := stringPatternConditionTokens(valueRef, prop); ok {
		if !isRequired {
			condition = wrapWithNullGuard(valueRef, condition)
		}
		condition = wrapWithNullGuard(parentRef, condition)
		appendValidation(varBody, condition, fmt.Sprintf("%s must match the pattern: %s.", displayName, prop.Constraints.Pattern))
	}

	// Arrays
	if condition, ok := arrayMinItemsConditionTokens(valueRef, prop); ok {
		if !isRequired {
			condition = wrapWithNullGuard(valueRef, condition)
		}
		condition = wrapWithNullGuard(parentRef, condition)
		appendValidation(varBody, condition, fmt.Sprintf("%s must have at least %d item(s).", displayName, *prop.Constraints.MinItems))
	}
	if condition, ok := arrayMaxItemsConditionTokens(valueRef, prop); ok {
		if !isRequired {
			condition = wrapWithNullGuard(valueRef, condition)
		}
		condition = wrapWithNullGuard(parentRef, condition)
		appendValidation(varBody, condition, fmt.Sprintf("%s must have at most %d item(s).", displayName, *prop.Constraints.MaxItems))
	}

	// Numbers
	if condition, msg, ok := numericMinimumConditionTokens(valueRef, prop, displayName); ok {
		if !isRequired {
			condition = wrapWithNullGuard(valueRef, condition)
		}
		condition = wrapWithNullGuard(parentRef, condition)
		appendValidation(varBody, condition, msg)
	}
	if condition, msg, ok := numericMaximumConditionTokens(valueRef, prop, displayName); ok {
		if !isRequired {
			condition = wrapWithNullGuard(valueRef, condition)
		}
		condition = wrapWithNullGuard(parentRef, condition)
		appendValidation(varBody, condition, msg)
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

func enumValuesForError(prop *schema.Property) []string {
	if prop == nil {
		return nil
	}
	values, ok := enumValues(prop)
	if !ok {
		return nil
	}
	return values
}

func enumValues(prop *schema.Property) ([]string, bool) {
	if prop == nil || len(prop.Enum) == 0 {
		return nil, false
	}
	values := make([]string, len(prop.Enum))
	copy(values, prop.Enum)
	sort.Strings(values)
	return values, true
}

func enumConditionTokens(valueRef hclwrite.Tokens, prop *schema.Property) (hclwrite.Tokens, bool) {
	values, ok := enumValues(prop)
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

func stringMinLengthConditionTokens(valueRef hclwrite.Tokens, prop *schema.Property) (hclwrite.Tokens, bool) {
	if prop == nil || prop.Type != schema.TypeString {
		return nil, false
	}
	if prop.Constraints.MinLength == nil || *prop.Constraints.MinLength <= 0 {
		return nil, false
	}
	lengthCall := hclwrite.TokensForFunctionCall("length", valueRef)
	var condition hclwrite.Tokens
	condition = append(condition, lengthCall...)
	condition = append(condition, &hclwrite.Token{Type: hclsyntax.TokenGreaterThanEq, Bytes: []byte(" >= ")})
	condition = append(condition, hclwrite.TokensForValue(cty.NumberIntVal(*prop.Constraints.MinLength))...)
	return condition, true
}

func stringMaxLengthConditionTokens(valueRef hclwrite.Tokens, prop *schema.Property) (hclwrite.Tokens, bool) {
	if prop == nil || prop.Type != schema.TypeString {
		return nil, false
	}
	if prop.Constraints.MaxLength == nil {
		return nil, false
	}
	lengthCall := hclwrite.TokensForFunctionCall("length", valueRef)
	var condition hclwrite.Tokens
	condition = append(condition, lengthCall...)
	condition = append(condition, &hclwrite.Token{Type: hclsyntax.TokenLessThanEq, Bytes: []byte(" <= ")})
	condition = append(condition, hclwrite.TokensForValue(cty.NumberIntVal(*prop.Constraints.MaxLength))...)
	return condition, true
}

func stringPatternConditionTokens(valueRef hclwrite.Tokens, prop *schema.Property) (hclwrite.Tokens, bool) {
	if prop == nil || prop.Type != schema.TypeString {
		return nil, false
	}
	if prop.Constraints.Pattern == "" {
		return nil, false
	}
	regexCall := hclwrite.TokensForFunctionCall("can",
		hclwrite.TokensForFunctionCall("regex",
			hclwrite.TokensForValue(cty.StringVal(prop.Constraints.Pattern)),
			valueRef,
		),
	)
	return regexCall, true
}

func arrayMinItemsConditionTokens(valueRef hclwrite.Tokens, prop *schema.Property) (hclwrite.Tokens, bool) {
	if prop == nil || prop.Type != schema.TypeArray {
		return nil, false
	}
	if prop.Constraints.MinItems == nil || *prop.Constraints.MinItems <= 0 {
		return nil, false
	}
	lengthCall := hclwrite.TokensForFunctionCall("length", valueRef)
	var condition hclwrite.Tokens
	condition = append(condition, lengthCall...)
	condition = append(condition, &hclwrite.Token{Type: hclsyntax.TokenGreaterThanEq, Bytes: []byte(" >= ")})
	condition = append(condition, hclwrite.TokensForValue(cty.NumberIntVal(*prop.Constraints.MinItems))...)
	return condition, true
}

func arrayMaxItemsConditionTokens(valueRef hclwrite.Tokens, prop *schema.Property) (hclwrite.Tokens, bool) {
	if prop == nil || prop.Type != schema.TypeArray {
		return nil, false
	}
	if prop.Constraints.MaxItems == nil {
		return nil, false
	}
	lengthCall := hclwrite.TokensForFunctionCall("length", valueRef)
	var condition hclwrite.Tokens
	condition = append(condition, lengthCall...)
	condition = append(condition, &hclwrite.Token{Type: hclsyntax.TokenLessThanEq, Bytes: []byte(" <= ")})
	condition = append(condition, hclwrite.TokensForValue(cty.NumberIntVal(*prop.Constraints.MaxItems))...)
	return condition, true
}

func numericMinimumConditionTokens(valueRef hclwrite.Tokens, prop *schema.Property, displayName string) (hclwrite.Tokens, string, bool) {
	if prop == nil || prop.Type != schema.TypeInteger {
		return nil, "", false
	}
	if prop.Constraints.MinValue == nil {
		return nil, "", false
	}
	var condition hclwrite.Tokens
	condition = append(condition, valueRef...)
	condition = append(condition, &hclwrite.Token{Type: hclsyntax.TokenGreaterThanEq, Bytes: []byte(" >= ")})
	condition = append(condition, hclwrite.TokensForValue(cty.NumberIntVal(*prop.Constraints.MinValue))...)
	return condition, fmt.Sprintf("%s must be greater than or equal to %d.", displayName, *prop.Constraints.MinValue), true
}

func numericMaximumConditionTokens(valueRef hclwrite.Tokens, prop *schema.Property, displayName string) (hclwrite.Tokens, string, bool) {
	if prop == nil || prop.Type != schema.TypeInteger {
		return nil, "", false
	}
	if prop.Constraints.MaxValue == nil {
		return nil, "", false
	}
	var condition hclwrite.Tokens
	condition = append(condition, valueRef...)
	condition = append(condition, &hclwrite.Token{Type: hclsyntax.TokenLessThanEq, Bytes: []byte(" <= ")})
	condition = append(condition, hclwrite.TokensForValue(cty.NumberIntVal(*prop.Constraints.MaxValue))...)
	return condition, fmt.Sprintf("%s must be less than or equal to %d.", displayName, *prop.Constraints.MaxValue), true
}

// generateEnumValidation generates validation for enum values.
func generateEnumValidation(varBody *hclwrite.Body, tfName string, prop *schema.Property, isRequired bool) {
	if prop == nil {
		return
	}

	varRef := hclgen.TokensForTraversal("var", tfName)
	condition, ok := enumConditionTokens(varRef, prop)
	if !ok {
		return
	}
	if !isRequired {
		condition = wrapWithNullGuard(varRef, condition)
	}
	appendValidation(varBody, condition, fmt.Sprintf("%s must be one of: %s.", tfName, joinEnumValues(enumValuesForError(prop))))
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
func generateStringValidations(varBody *hclwrite.Body, tfName string, prop *schema.Property, isRequired bool) {
	if prop == nil || prop.Type != schema.TypeString {
		return
	}

	varRef := hclgen.TokensForTraversal("var", tfName)

	if condition, ok := stringMinLengthConditionTokens(varRef, prop); ok {
		if !isRequired {
			condition = wrapWithNullGuard(varRef, condition)
		}
		appendValidation(varBody, condition, fmt.Sprintf("%s must have a minimum length of %d.", tfName, *prop.Constraints.MinLength))
	}

	if condition, ok := stringMaxLengthConditionTokens(varRef, prop); ok {
		if !isRequired {
			condition = wrapWithNullGuard(varRef, condition)
		}
		appendValidation(varBody, condition, fmt.Sprintf("%s must have a maximum length of %d.", tfName, *prop.Constraints.MaxLength))
	}

	if condition, ok := stringPatternConditionTokens(varRef, prop); ok {
		if !isRequired {
			condition = wrapWithNullGuard(varRef, condition)
		}
		appendValidation(varBody, condition, fmt.Sprintf("%s must match the pattern: %s.", tfName, prop.Constraints.Pattern))
	}
}

// generateArrayValidations generates validation for array/list constraints.
func generateArrayValidations(varBody *hclwrite.Body, tfName string, prop *schema.Property, isRequired bool) {
	if prop == nil || prop.Type != schema.TypeArray {
		return
	}

	varRef := hclgen.TokensForTraversal("var", tfName)

	if condition, ok := arrayMinItemsConditionTokens(varRef, prop); ok {
		if !isRequired {
			condition = wrapWithNullGuard(varRef, condition)
		}
		appendValidation(varBody, condition, fmt.Sprintf("%s must have at least %d item(s).", tfName, *prop.Constraints.MinItems))
	}

	if condition, ok := arrayMaxItemsConditionTokens(varRef, prop); ok {
		if !isRequired {
			condition = wrapWithNullGuard(varRef, condition)
		}
		appendValidation(varBody, condition, fmt.Sprintf("%s must have at most %d item(s).", tfName, *prop.Constraints.MaxItems))
	}
}

// generateNumericValidations generates validation for numeric constraints.
func generateNumericValidations(varBody *hclwrite.Body, tfName string, prop *schema.Property, isRequired bool) {
	if prop == nil || prop.Type != schema.TypeInteger {
		return
	}

	varRef := hclgen.TokensForTraversal("var", tfName)

	if condition, msg, ok := numericMinimumConditionTokens(varRef, prop, tfName); ok {
		if !isRequired {
			condition = wrapWithNullGuard(varRef, condition)
		}
		appendValidation(varBody, condition, msg)
	}

	if condition, msg, ok := numericMaximumConditionTokens(varRef, prop, tfName); ok {
		if !isRequired {
			condition = wrapWithNullGuard(varRef, condition)
		}
		appendValidation(varBody, condition, msg)
	}
}
