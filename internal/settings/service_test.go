package settings

import "testing"

func TestValidateMemoryProviderClientType(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{name: "openai", input: "openai", wantErr: false},
		{name: "openai compat", input: "openai-compat", wantErr: false},
		{name: "anthropic", input: "anthropic", wantErr: false},
		{name: "google", input: "google", wantErr: false},
		{name: "unsupported azure", input: "azure", wantErr: true},
		{name: "unsupported empty", input: "", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateMemoryProviderClientType(tt.input)
			if tt.wantErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
