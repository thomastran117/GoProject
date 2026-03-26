package auth

const (
	// RolePending is the zero-value assigned to OAuth-created users who have not
	// yet completed role selection. It is intentionally the empty string so that
	// the JWT payload signals "pending" to the frontend without extra fields.
	RolePending = ""

	RoleStudent           = "student"
	RoleTeacher           = "teacher"
	RolePrincipal         = "principal"
	RoleTeachingAssistant = "teaching_assistant"
	RoleAdmin             = "admin"
)

var validSignupRoles = map[string]bool{
	RoleStudent:           true,
	RoleTeacher:           true,
	RolePrincipal:         true,
	RoleTeachingAssistant: true,
}

// IsValidSignupRole reports whether role is one of the four roles a user may
// self-assign at signup or after OAuth. Admin is intentionally excluded.
func IsValidSignupRole(role string) bool {
	return validSignupRoles[role]
}
