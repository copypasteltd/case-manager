package core

import "testing"

func TestStorageNameEqualityAndUnsafePaths(t *testing.T) {
	for _, pair := range [][2]string{
		{"Résumé.PDF", "Re\u0301sume\u0301.pdf"},
		{"合同Ａ.PDF", "合同a.pdf"},
		{"Straße.TXT", "STRASSE.txt"},
	} {
		a, err := StorageNameKey(pair[0])
		if err != nil {
			t.Fatal(err)
		}
		b, err := StorageNameKey(pair[1])
		if err != nil || a != b {
			t.Fatalf("equivalent names have different keys: %q, %q", a, b)
		}
	}
	for _, name := range []string{"", " ", ".", "..", "../财务", "a\\b", "／etc", "a\u202eb", "a\x00b", "con.txt", "ＣＯＭ１.pdf", "lpt9", "a.", " a", "a ", "a:b", "a?b", "a*b", "a|b"} {
		if _, err := StorageNameKey(name); err == nil {
			t.Errorf("unsafe name accepted: %q", name)
		}
	}
	if _, err := StorageNameKey("项目·成品（终稿）🎬.mp4"); err != nil {
		t.Fatal(err)
	}
}
