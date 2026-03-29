package validators

import (
	"unicode"

	"github.com/go-playground/validator/v10"
)

var validSignupRoles = map[string]bool{
	"student": true, "teacher": true, "principal": true, "teaching_assistant": true,
}

var registry = map[string]validator.Func{
	"strong_password":   strongPassword,
	"valid_signup_role": validSignupRole,
}

func validSignupRole(fl validator.FieldLevel) bool {
	return validSignupRoles[fl.Field().String()]
}

func Register(v *validator.Validate) {
	for name, fn := range registry {
		v.RegisterValidation(name, fn)
	}
}

func strongPassword(fl validator.FieldLevel) bool {
	var hasUpper, hasLower, hasDigit, hasSpecial bool
	for _, ch := range fl.Field().String() {
		switch {
		case unicode.IsUpper(ch):
			hasUpper = true
		case unicode.IsLower(ch):
			hasLower = true
		case unicode.IsDigit(ch):
			hasDigit = true
		case unicode.IsPunct(ch) || unicode.IsSymbol(ch):
			hasSpecial = true
		}
	}
	return hasUpper && hasLower && hasDigit && hasSpecial
}
