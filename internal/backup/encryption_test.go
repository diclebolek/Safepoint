// =============================================================================
// Bu dosya ne ise yarar? Encryption extension / secret parse testleri.
// =============================================================================

package backup_test

import (
	"testing"

	"github.com/diclebolek/Safepoint/internal/backup"
)

func TestEncryptedExtension(t *testing.T) {
	if got := backup.EncryptedExtension("dump.gz", false); got != "dump.gz" {
		t.Fatalf("got %q", got)
	}
	if got := backup.EncryptedExtension("dump.gz", true); got != "dump.gz.enc" {
		t.Fatalf("got %q", got)
	}
}

func TestParseEncryptionSecret(t *testing.T) {
	pass, err := backup.ParseEncryptionSecret(map[string][]byte{"password": []byte("s3cret")})
	if err != nil || pass != "s3cret" {
		t.Fatalf("password field: %v %q", err, pass)
	}
	pass, err = backup.ParseEncryptionSecret(map[string][]byte{"key": []byte("k")})
	if err != nil || pass != "k" {
		t.Fatalf("key field: %v %q", err, pass)
	}
	if _, err := backup.ParseEncryptionSecret(map[string][]byte{}); err == nil {
		t.Fatal("expected error")
	}
}
