package statusbridge

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"math/big"
	"net/http"
	"net/url"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

type sample struct {
	ObservedAt time.Time `json:"observed_at"`
	Healthy    bool      `json:"healthy"`
	LatencyMS  int64     `json:"latency_ms"`
}

func (s sample) digest() string {
	b, _ := json.Marshal(s)
	d := sha256.Sum256(b)
	return hex.EncodeToString(d[:])
}

func labels(t Target) map[string]string {
	out := map[string]string{"job": t.Job, "instance": t.Instance}
	for k, v := range t.Labels {
		out[k] = v
	}
	return out
}

func selector(metric string, t Target) string {
	m := labels(t)
	names := make([]string, 0, len(m))
	for name := range m {
		names = append(names, name)
	}
	sort.Strings(names)
	parts := make([]string, 0, len(names))
	for _, name := range names {
		parts = append(parts, name+"="+strconv.Quote(m[name]))
	}
	return metric + "{" + strings.Join(parts, ",") + "}"
}

// Prometheus evaluates selectors at query time. timestamp(selector) returns
// the actual scrape timestamp, preventing old lookback values becoming fresh.
func fetchSample(ctx context.Context, client *http.Client, base *url.URL, bearer string, t Target, now time.Time, maxAge int) (sample, error) {
	now = now.UTC().Truncate(time.Millisecond)
	values := map[string]float64{}
	var observed time.Time
	for _, name := range []string{"up", "probe_success", "probe_duration_seconds"} {
		q := selector(name, t)
		v, err := query(ctx, client, base, bearer, q, name, t, now)
		if err != nil {
			return sample{}, err
		}
		raw, err := query(ctx, client, base, bearer, "timestamp("+q+")", "", t, now)
		if err != nil {
			return sample{}, err
		}
		at, err := millisecondTime(raw)
		if err != nil || at.After(now) || now.Sub(at) >= time.Duration(maxAge)*time.Second || !observed.IsZero() && !at.Equal(observed) {
			return sample{}, fault("SAMPLE_TIMESTAMP_INVALID")
		}
		observed = at
		n, err := strconv.ParseFloat(v, 64)
		if err != nil || math.IsNaN(n) || math.IsInf(n, 0) {
			return sample{}, fault("SAMPLE_VALUE_INVALID")
		}
		values[name] = n
	}
	if values["up"] != 1 {
		return sample{}, fault("SCRAPE_UNAVAILABLE")
	}
	healthy, duration := values["probe_success"], values["probe_duration_seconds"]
	if healthy != 0 && healthy != 1 || duration < 0 || duration > 3600 {
		return sample{}, fault("SAMPLE_VALUE_INVALID")
	}
	return sample{ObservedAt: observed, Healthy: healthy == 1, LatencyMS: int64(math.Ceil(duration * 1000))}, nil
}

func millisecondTime(raw string) (time.Time, error) {
	if len(raw) > 40 || !regexp.MustCompile(`^[0-9]{1,12}(?:\.[0-9]{1,12})?(?:[eE][+-]?[0-9]{1,2})?$`).MatchString(raw) {
		return time.Time{}, fault("SAMPLE_TIMESTAMP_INVALID")
	}
	r, ok := new(big.Rat).SetString(raw)
	if !ok {
		return time.Time{}, fault("SAMPLE_TIMESTAMP_INVALID")
	}
	r.Mul(r, big.NewRat(1000, 1))
	if !r.IsInt() || !r.Num().IsInt64() || r.Sign() <= 0 {
		return time.Time{}, fault("SAMPLE_TIMESTAMP_INVALID")
	}
	return time.UnixMilli(r.Num().Int64()).UTC(), nil
}

func query(ctx context.Context, client *http.Client, base *url.URL, bearer, q, metric string, t Target, now time.Time) (string, error) {
	u := *base
	u.Path += "/api/v1/query"
	u.RawQuery = url.Values{"query": {q}, "time": {now.Format(time.RFC3339Nano)}}.Encode()
	r, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if err != nil {
		return "", fault("QUERY_FAILED")
	}
	r.Header.Set("Accept", "application/json")
	if bearer != "" {
		r.Header.Set("Authorization", "Bearer "+bearer)
	}
	res, err := client.Do(r)
	if err != nil {
		return "", fault("QUERY_FAILED")
	}
	body, err := responseBody(res, 64<<10)
	if err != nil || res.StatusCode != 200 || uniqueJSON(body) != nil {
		return "", fault("QUERY_RESPONSE_INVALID")
	}
	var wire struct {
		Status   string   `json:"status"`
		Warnings []string `json:"warnings"`
		Error    string   `json:"error"`
		Data     struct {
			ResultType string `json:"resultType"`
			Result     []struct {
				Metric    map[string]string `json:"metric"`
				Value     []json.RawMessage `json:"value"`
				Histogram json.RawMessage   `json:"histogram"`
			} `json:"result"`
		} `json:"data"`
	}
	if json.Unmarshal(body, &wire) != nil || wire.Status != "success" || len(wire.Warnings) != 0 || wire.Error != "" || wire.Data.ResultType != "vector" || len(wire.Data.Result) != 1 {
		return "", fault("QUERY_RESPONSE_INVALID")
	}
	row := wire.Data.Result[0]
	if len(row.Histogram) != 0 || len(row.Value) != 2 {
		return "", fault("QUERY_RESPONSE_INVALID")
	}
	if name, present := row.Metric["__name__"]; present && name != metric {
		return "", fault("SAMPLE_LABELS_INVALID")
	}
	delete(row.Metric, "__name__")
	if !reflect.DeepEqual(row.Metric, labels(t)) {
		return "", fault("SAMPLE_LABELS_INVALID")
	}
	stamp, err := millisecondTime(string(row.Value[0]))
	var value string
	if err != nil || !stamp.Equal(now) || json.Unmarshal(row.Value[1], &value) != nil || value == "" {
		return "", fault("QUERY_RESPONSE_INVALID")
	}
	return value, nil
}
