package files

import "testing"

func TestDownloadRangeBoundsDoNotFallbackToFullObject(t *testing.T) {
	for _, input := range []struct {
		value string
		size  int64
	}{
		{"", 120000000000}, {"bytes=0-33554432", 120000000000}, {"bytes=0-", 120000000000},
		{"bytes=-20", 120000000000}, {"bytes=0-1,4-5", 120000000000}, {"bytes=0-20", 20},
		{"bytes=5-4", 20}, {"bytes=+0-1", 20}, {"bytes=00-1", 20},
		{"bytes=0-9223372036854775808", 120000000000},
	} {
		if _, _, _, err := DownloadRange(input.value, input.size); err == nil {
			t.Fatal("unbounded or ambiguous download accepted", input.value)
		}
	}
	offset, length, partial, err := DownloadRange("bytes=119966445568-119999999999", 120000000000)
	if err != nil || !partial || offset != 119966445568 || length != DownloadRangeBytes {
		t.Fatal("last 32MiB of 120GB version was not parsed", offset, length, err)
	}
	if _, length, partial, err = DownloadRange("", 0); err != nil || length != 0 || partial {
		t.Fatal("empty retained object was not handled")
	}
}
