package config

import "testing"

func TestValidateVersion(t *testing.T) {
	tests := []struct {
		version string
		valid   bool
	}{
		{"1.0.0", true},
		{"0.137.0", true},
		{"2026.06.04-5fd875e", true},
		{"v1.2.3", true},
		{"", false},
		{".", false},
		{"..", false},
		{"../../etc", false},
		{"a/b", false},
		{"foo/../bar", false},
		{`..\windows`, false},
		{"with space", false},
	}

	for _, tt := range tests {
		t.Run(tt.version, func(t *testing.T) {
			// act
			err := ValidateVersion(tt.version)

			// assert
			if (err == nil) != tt.valid {
				t.Errorf("ValidateVersion(%q) error = %v, want valid=%v", tt.version, err, tt.valid)
			}
		})
	}
}
