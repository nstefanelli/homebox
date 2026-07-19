package hasher

import (
	"encoding/base64"
	"fmt"
	"strings"
	"testing"
)

func TestDecodeHashRejectsResourceExhaustionParameters(t *testing.T) {
	validSalt := base64.RawStdEncoding.EncodeToString([]byte("0123456789abcdef"))
	validKey := base64.RawStdEncoding.EncodeToString(make([]byte, 32))

	tests := []struct {
		name string
		hash string
	}{
		{
			name: "excessive memory",
			hash: fmt.Sprintf("$argon2id$v=19$m=%d,t=3,p=2$%s$%s", maxArgonMemoryKiB+1, validSalt, validKey),
		},
		{
			name: "zero iterations",
			hash: fmt.Sprintf("$argon2id$v=19$m=65536,t=0,p=2$%s$%s", validSalt, validKey),
		},
		{
			name: "oversized salt",
			hash: fmt.Sprintf("$argon2id$v=19$m=65536,t=3,p=2$%s$%s",
				base64.RawStdEncoding.EncodeToString([]byte(strings.Repeat("s", maxArgonComponentLength+1))),
				validKey),
		},
		{
			name: "oversized key",
			hash: fmt.Sprintf("$argon2id$v=19$m=65536,t=3,p=2$%s$%s",
				validSalt,
				base64.RawStdEncoding.EncodeToString([]byte(strings.Repeat("k", maxArgonComponentLength+1)))),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, _, err := decodeHash(tt.hash)
			if err == nil {
				t.Fatal("decodeHash accepted resource-exhausting parameters")
			}
		})
	}
}

func TestHashPassword(t *testing.T) {
	t.Parallel()
	type args struct {
		password      string
		invalidInputs []string
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "letters_and_numbers",
			args: args{
				password:      "password123456788",
				invalidInputs: []string{"testPassword", "AnotherBadPassword", "ThisShouldNeverWork", "1234567890"},
			},
		},
		{
			name: "letters_number_and_special",
			args: args{
				password:      "!2afj3214pofajip3142j;fa",
				invalidInputs: []string{"testPassword", "AnotherBadPassword", "ThisShouldNeverWork", "1234567890"},
			},
		},
		{
			name: "extra_long_password",
			args: args{
				password:      "this_is_a_very_long_password_that_should_be_hashed_properly_and_still_work_with_the_check_function",
				invalidInputs: []string{"testPassword", "AnotherBadPassword", "ThisShouldNeverWork", "1234567890"},
			},
		},
		{
			name: "empty_password",
			args: args{
				password:      "",
				invalidInputs: []string{"testPassword", "AnotherBadPassword", "ThisShouldNeverWork", "1234567890"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := HashPassword(tt.args.password)
			if (err != nil) != tt.wantErr {
				t.Errorf("HashPassword() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			check, _ := CheckPasswordHash(tt.args.password, got)
			if !check {
				t.Errorf("CheckPasswordHash() failed to validate password=%v against hash=%v", tt.args.password, got)
			}

			for _, invalid := range tt.args.invalidInputs {
				check, _ := CheckPasswordHash(invalid, got)
				if check {
					t.Errorf("CheckPasswordHash() improperly validated password=%v against hash=%v", invalid, got)
				}
			}
		})
	}
}
