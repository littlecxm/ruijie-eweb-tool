package auth

import "testing"

func TestSshPasswdGenerate(t *testing.T) {
	tests := []struct {
		sn   string
		want string
	}{
		{"H1V0226010131", "19da49631999b849"},
		{"", "258e55d0e805d844"},
		{"ABC123", "91702b894b34b39a"},
		{"ABC123\n", "96a200d6fadb29b1"},
		{"abc123", "358888a7202c2b17"},
		{"1234567890123", "bba8c6ad16bad416"},
		{"ABC123\x00ignored", "91702b894b34b39a"},
		{"1234567890123\x00ignored", "bba8c6ad16bad416"},
	}
	for _, tt := range tests {
		t.Run(tt.sn, func(t *testing.T) {
			got, err := SshPasswdGenerate(tt.sn)
			if err != nil || got != tt.want {
				t.Fatalf("SshPasswdGenerate(%q) = %q, %v; want %q", tt.sn, got, err, tt.want)
			}
		})
	}
}

func TestPasswdGenerateRejectsUnterminatedBuffer(t *testing.T) {
	for _, sn := range []string{"12345678901234", "12345678901234567890", "12345678901234\x00"} {
		if got, err := SshPasswdGenerate(sn); err == nil || got != "" {
			t.Fatalf("SshPasswdGenerate(%q) = %q, %v; want an error", sn, got, err)
		}
	}
}
