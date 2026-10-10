package main

import (
	"bytes"
	"strings"
	"testing"

	identityinfra "github.com/CarlosSV923/VideoExplorer/apps/api/internal/identity/infrastructure"
)

func TestHashPassword(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{name: "line with a newline", input: "pw\n"},
		{name: "no trailing newline", input: "pw"},
		{name: "windows line ending", input: "pw\r\n"},
		{name: "empty line", input: "\n", wantErr: true},
		{name: "nothing on stdin", input: "", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var out bytes.Buffer
			err := hashPassword(strings.NewReader(tt.input), &out)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			v, err := identityinfra.NewArgon2idVerifier(strings.TrimSpace(out.String()))
			if err != nil {
				t.Fatalf("printed hash is not a valid argon2id PHC string: %v", err)
			}
			if !v.Verify("pw") || v.Verify("pw ") {
				t.Fatal("printed hash does not verify the password read from stdin")
			}
		})
	}
}
