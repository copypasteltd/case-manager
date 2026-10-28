package nativeruntime

import "strings"

func parseMounts(raw string) ([]Mount, error) {
	var mounts []Mount
	for _, line := range strings.Split(strings.TrimSpace(raw), "\n") {
		left, right, ok := strings.Cut(line, " - ")
		a, b := strings.Fields(left), strings.Fields(right)
		if !ok || len(a) < 6 || len(b) < 3 {
			return nil, invalid()
		}
		unescape := strings.NewReplacer(`\040`, " ", `\011`, "\t", `\012`, "\n", `\134`, `\`)
		mounts = append(mounts, Mount{Target: unescape.Replace(a[4]), Type: b[0], Options: strings.Split(a[5], ","), SuperOptions: strings.Split(b[2], ",")})
	}
	return mounts, nil
}
