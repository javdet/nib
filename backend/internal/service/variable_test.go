package service

import (
	"errors"
	"testing"

	"github.com/javdet/nib/internal/domain"
)

func TestNormalizeVariable_emptyScopeDefaultsToGlobal(t *testing.T) {
	v := normalizeVariable(domain.PromptVariable{Name: "CompanyName"})
	if v.Scope != defaultVariableScope {
		t.Fatalf("scope = %q, want %q", v.Scope, defaultVariableScope)
	}
	if v.Kind != VariableKindString {
		t.Fatalf("kind = %q, want %q", v.Kind, VariableKindString)
	}
}

func TestValidateVariable(t *testing.T) {
	tests := []struct {
		name    string
		v       domain.PromptVariable
		wantErr error
	}{
		{
			name: "valid",
			v:    domain.PromptVariable{Scope: "global", Name: "CompanyName"},
		},
		{
			name:    "empty name",
			v:       domain.PromptVariable{Scope: "global", Name: ""},
			wantErr: ErrInvalidVariableName,
		},
		{
			name:    "invalid name chars",
			v:       domain.PromptVariable{Scope: "global", Name: "bad name"},
			wantErr: ErrInvalidVariableName,
		},
		{
			name:    "dot in name",
			v:       domain.PromptVariable{Scope: "global", Name: "foo.bar"},
			wantErr: ErrInvalidVariableName,
		},
		{
			name:    "empty scope after normalize still invalid",
			v:       domain.PromptVariable{Scope: "   ", Name: "X"},
			wantErr: nil, // normalize sets global
		},
		{
			name:    "invalid scope",
			v:       domain.PromptVariable{Scope: "bad scope", Name: "X"},
			wantErr: ErrInvalidVariableScope,
		},
		{
			name:    "dot in scope",
			v:       domain.PromptVariable{Scope: "foo.bar", Name: "X"},
			wantErr: ErrInvalidVariableScope,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := normalizeVariable(tt.v)
			err := validateVariable(v)
			if tt.wantErr == nil {
				if tt.name == "empty scope after normalize still invalid" {
					if err != nil {
						t.Fatalf("validateVariable() error = %v, want nil", err)
					}
					if v.Scope != defaultVariableScope {
						t.Fatalf("scope = %q, want %q", v.Scope, defaultVariableScope)
					}
					return
				}
				if err != nil {
					t.Fatalf("validateVariable() error = %v, want nil", err)
				}
				return
			}
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("validateVariable() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateVariable_listValue(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{name: "valid list", value: `["a","b"]`},
		{name: "empty list", value: `[]`},
		{name: "invalid json", value: `not-json`, wantErr: true},
		{name: "non-string array", value: `[1,2]`, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateVariable(domain.PromptVariable{
				Scope: "global",
				Name:  "toolCategories",
				Kind:  VariableKindList,
				Value: tt.value,
			})
			if tt.wantErr {
				if !errors.Is(err, ErrInvalidVariableValue) {
					t.Fatalf("validateVariable() error = %v, want %v", err, ErrInvalidVariableValue)
				}
				return
			}
			if err != nil {
				t.Fatalf("validateVariable() error = %v, want nil", err)
			}
		})
	}
}

func TestValidateVariable_invalidKind(t *testing.T) {
	err := validateVariable(domain.PromptVariable{
		Scope: "global",
		Name:  "X",
		Kind:  "map",
		Value: "value",
	})
	if !errors.Is(err, ErrInvalidVariableValue) {
		t.Fatalf("validateVariable() error = %v, want %v", err, ErrInvalidVariableValue)
	}
}
