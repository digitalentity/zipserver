package auth

import "testing"

func TestIsUserAllowed(t *testing.T) {
	tests := []struct {
		name         string
		allowedUsers []string
		email        string
		want         bool
	}{
		{
			name:         "Exact match",
			allowedUsers: []string{"user@example.com"},
			email:        "user@example.com",
			want:         true,
		},
		{
			name:         "Domain wildcard",
			allowedUsers: []string{"*@company.com"},
			email:        "dev@company.com",
			want:         true,
		},
		{
			name:         "Domain wildcard mismatch",
			allowedUsers: []string{"*@company.com"},
			email:        "dev@other.com",
			want:         false,
		},
		{
			name:         "All users",
			allowedUsers: []string{"*"},
			email:        "any@thing.com",
			want:         true,
		},
		{
			name:         "No match",
			allowedUsers: []string{"admin@example.com"},
			email:        "user@example.com",
			want:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &Authenticator{allowedUsers: tt.allowedUsers}
			if got := a.IsUserAllowed(tt.email); got != tt.want {
				t.Errorf("IsUserAllowed() = %v, want %v", got, tt.want)
			}
		})
	}
}
