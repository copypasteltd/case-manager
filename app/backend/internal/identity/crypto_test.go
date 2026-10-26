package identity

import (
	"encoding/base32"
	"testing"
	"time"
)

func TestTOTPRFC6238VectorsAndBinding(t *testing.T) {
	secret := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString([]byte("12345678901234567890"))
	for _, test := range []struct {
		at   int64
		code string
	}{{59, "287082"}, {1111111109, "081804"}, {1111111111, "050471"}, {1234567890, "005924"}, {2000000000, "279037"}, {20000000000, "353130"}} {
		if _, valid := MatchTOTP(secret, test.code, time.Unix(test.at, 0)); !valid {
			t.Errorf("RFC vector failed at %d", test.at)
		}
		if _, valid := MatchTOTP(secret, test.code, time.Unix(test.at+120, 0)); valid {
			t.Errorf("accepted expired code at %d", test.at)
		}
	}
	key := []byte("01234567890123456789012345678901")
	encrypted, err := Seal(key, secret, "org:user")
	if err != nil {
		t.Fatal(err)
	}
	if value, err := Open(key, encrypted, "org:user"); err != nil || value != secret {
		t.Fatal("cannot recover bound credential")
	}
	if _, err := Open(key, encrypted, "org:another"); err == nil {
		t.Fatal("accepted credential under another identity")
	}
	encrypted[len(encrypted)-1] ^= 1
	if _, err := Open(key, encrypted, "org:user"); err == nil {
		t.Fatal("accepted modified credential")
	}
}
func TestArgon2PasswordVerification(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if !CheckPassword(hash, "correct horse battery staple") {
		t.Fatal("valid password rejected")
	}
	if CheckPassword(hash, "incorrect horse battery staple") {
		t.Fatal("wrong password accepted")
	}
	if _, err := HashPassword("short"); err == nil {
		t.Fatal("short credential accepted")
	}
	if CheckPassword("$argon2id$v=19$m=4294967295,t=3,p=2$c2FsdA$YWJj", "a") {
		t.Fatal("unbounded KDF accepted")
	}
}
