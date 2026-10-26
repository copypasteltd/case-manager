// Package filescan scans a supplied immutable byte stream. It never executes
// file paths, extracts archives locally or treats an unavailable engine as clean.
package filescan

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type Result struct {
	State             string `json:"state"`
	Code              string `json:"code"`
	Engine            string `json:"engine"`
	EngineVersion     string `json:"engine_version"`
	SignatureRevision string `json:"signature_revision"`
	PolicySHA256      string `json:"policy_sha256"`
	StreamedBytes     int64  `json:"streamed_bytes"`
}
type Scanner interface {
	// Scan owns and closes source, including when skipped or cancelled.
	Scan(context.Context, io.ReadCloser, int64) (Result, error)
}
type Error struct {
	Code      string
	Retryable bool
}

func (e *Error) Error() string              { return e.Code }
func failure(code string, retry bool) error { return &Error{Code: code, Retryable: retry} }

type Policy struct {
	MaxBytes int64
	Timeout  time.Duration
	SHA256   string
	// An operator must attest daemon limits, AlertExceedsMax and encrypted /
	// unsupported-content handling against this policy before accepting OK.
	LimitsAttested bool
}

// Conservative decimal ceiling below ClamAV's documented 2 GB design limit.
// Larger objects need another engine or a separately audited waiver.
const MaxClamAVBytes int64 = 2000000000

type Config struct {
	Network, Address string
	Policy           Policy
}

func (c Config) String() string   { return "filescan.Config{connection:redacted}" }
func (c Config) GoString() string { return c.String() }

type Client struct{ config Config }

var digestPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)
var versionPattern = regexp.MustCompile(`^ClamAV ([0-9A-Za-z.+_-]{1,80})/([0-9]{1,20})/[^\r\n]{1,120}$`)

func New(c Config) (*Client, error) {
	if c.Network != "tcp" && c.Network != "unix" {
		return nil, failure("SCAN_CONFIGURATION_INVALID", false)
	}
	if c.Network == "tcp" {
		host, port, err := net.SplitHostPort(c.Address)
		number, e := strconv.Atoi(port)
		ip := net.ParseIP(host)
		if err != nil || e != nil || number < 1 || number > 65535 || ip == nil || !ip.IsLoopback() {
			return nil, failure("SCAN_LOCAL_ENDPOINT_REQUIRED", false)
		}
	} else if !filepath.IsAbs(c.Address) || filepath.Clean(c.Address) != c.Address || strings.ContainsAny(c.Address, "\x00\r\n") {
		return nil, failure("SCAN_LOCAL_ENDPOINT_REQUIRED", false)
	}
	p := c.Policy
	if p.MaxBytes < 1 || p.MaxBytes > MaxClamAVBytes || p.Timeout < time.Second || p.Timeout > 15*time.Minute || !digestPattern.MatchString(p.SHA256) {
		return nil, failure("SCAN_CONFIGURATION_INVALID", false)
	}
	if !p.LimitsAttested {
		return nil, failure("SCAN_POLICY_NOT_ATTESTED", false)
	}
	return &Client{config: c}, nil
}

type Disabled struct{}

func (Disabled) Scan(ctx context.Context, source io.ReadCloser, size int64) (Result, error) {
	if source != nil {
		_ = source.Close()
	}
	return Result{State: "unscannable", Code: "SCANNER_NOT_CONFIGURED"}, nil
}

type engine struct{ version, signatures string }

func (c *Client) dial(ctx context.Context) (net.Conn, error) {
	conn, err := (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, c.config.Network, c.config.Address)
	if err != nil {
		return nil, failure("SCAN_TRANSPORT_FAILED", true)
	}
	if deadline, ok := ctx.Deadline(); ok {
		if err = conn.SetDeadline(deadline); err != nil {
			conn.Close()
			return nil, failure("SCAN_TRANSPORT_FAILED", true)
		}
	}
	return conn, nil
}
func writeAll(conn net.Conn, data []byte) error {
	for len(data) > 0 {
		n, err := conn.Write(data)
		if err != nil {
			return failure("SCAN_TRANSPORT_FAILED", true)
		}
		if n < 1 {
			return failure("SCAN_TRANSPORT_FAILED", true)
		}
		data = data[n:]
	}
	return nil
}

// Every command uses a dedicated connection; a bounded complete reply is
// required. Ambiguous multiple records and oversized diagnostics are rejected.
func reply(conn net.Conn) (string, error) {
	raw, err := io.ReadAll(io.LimitReader(conn, 4097))
	if err != nil {
		return "", failure("SCAN_TRANSPORT_FAILED", true)
	}
	if len(raw) < 1 || len(raw) > 4096 || raw[len(raw)-1] != 0 {
		return "", failure("SCAN_INVALID_REPLY", false)
	}
	record := string(raw[:len(raw)-1])
	if strings.ContainsAny(record, "\x00\r\n") {
		return "", failure("SCAN_INVALID_REPLY", false)
	}
	return record, nil
}
func (c *Client) engine(ctx context.Context) (engine, error) {
	conn, err := c.dial(ctx)
	if err != nil {
		return engine{}, err
	}
	defer conn.Close()
	stop := context.AfterFunc(ctx, func() { conn.Close() })
	defer stop()
	if err = writeAll(conn, []byte("zVERSIONCOMMANDS\x00")); err != nil {
		return engine{}, err
	}
	raw, err := reply(conn)
	if err != nil {
		return engine{}, err
	}
	parts := strings.SplitN(raw, "|", 2)
	if len(parts) != 2 {
		return engine{}, failure("SCAN_CAPABILITIES_UNAVAILABLE", false)
	}
	match := versionPattern.FindStringSubmatch(strings.TrimSpace(parts[0]))
	commands := strings.Fields(strings.TrimSpace(parts[1]))
	supported := false
	if len(commands) > 0 && commands[0] == "COMMANDS:" {
		for _, command := range commands[1:] {
			if command == "INSTREAM" {
				supported = true
			}
		}
	}
	if len(match) != 3 || !supported {
		return engine{}, failure("SCAN_CAPABILITIES_UNAVAILABLE", false)
	}
	return engine{version: match[1], signatures: match[2]}, nil
}
func (c *Client) Scan(ctx context.Context, source io.ReadCloser, size int64) (result Result, err error) {
	if source == nil || size < 0 || size > 120000000000 {
		if source != nil {
			_ = source.Close()
		}
		return result, failure("SCAN_SOURCE_INVALID", false)
	}
	defer func() {
		if closeErr := source.Close(); closeErr != nil && err == nil {
			result = Result{}
			err = failure("SCAN_SOURCE_CLOSE_FAILED", true)
		}
	}()
	result = Result{State: "unscannable", Engine: "clamav", PolicySHA256: c.config.Policy.SHA256}
	if size > c.config.Policy.MaxBytes {
		result.Code = "SCAN_SIZE_LIMIT"
		return result, nil
	}
	ctx, cancel := context.WithTimeout(ctx, c.config.Policy.Timeout)
	defer cancel()
	stopSource := context.AfterFunc(ctx, func() { source.Close() })
	defer stopSource()
	before, err := c.engine(ctx)
	if err != nil {
		return Result{}, err
	}
	result.EngineVersion = before.version
	result.SignatureRevision = before.signatures
	conn, err := c.dial(ctx)
	if err != nil {
		return Result{}, err
	}
	defer conn.Close()
	stopConn := context.AfterFunc(ctx, func() { conn.Close() })
	defer stopConn()
	if err = writeAll(conn, []byte("zINSTREAM\x00")); err != nil {
		return Result{}, err
	}
	count, err := send(ctx, conn, source, size)
	if err != nil {
		// A daemon may reject its own stream limit before the client finished
		// writing. Accept only that explicit bounded diagnostic as unscannable.
		var own *Error
		if errors.As(err, &own) && own.Code == "SCAN_TRANSPORT_FAILED" {
			if raw, e := reply(conn); e == nil && strings.Contains(raw, "INSTREAM size limit exceeded") {
				result.Code = "SCAN_ENGINE_SIZE_LIMIT"
				return result, nil
			}
		}
		return Result{}, err
	}
	raw, err := reply(conn)
	if err != nil {
		return Result{}, err
	}
	result.StreamedBytes = count
	switch {
	case raw == "stream: OK":
		after, err := c.engine(ctx)
		if err != nil {
			return Result{}, err
		}
		if after != before {
			result.Code = "SCAN_ENGINE_CHANGED"
			return result, nil
		}
		if ctx.Err() != nil {
			return Result{}, failure("SCAN_TRANSPORT_FAILED", true)
		}
		result.State = "clean"
		result.Code = "SCAN_PASSED"
		return result, nil
	case strings.HasPrefix(raw, "stream: ") && strings.HasSuffix(raw, " FOUND"):
		if strings.Contains(raw, "Heuristics.Limits.Exceeded") || strings.Contains(raw, "Heuristics.Encrypted.") {
			result.Code = "SCAN_CONTENT_LIMIT"
			return result, nil
		}
		result.State = "quarantined"
		result.Code = "MALWARE_DETECTED"
		return result, nil
	case strings.Contains(raw, "INSTREAM size limit exceeded"):
		result.Code = "SCAN_ENGINE_SIZE_LIMIT"
		return result, nil
	case raw == "COMMAND UNAVAILABLE" || raw == "UNKNOWN COMMAND":
		result.Code = "SCAN_COMMAND_UNAVAILABLE"
		return result, nil
	case strings.HasSuffix(raw, " ERROR"):
		result.Code = "SCAN_ENGINE_ERROR"
		return result, nil
	default:
		return Result{}, failure("SCAN_INVALID_REPLY", false)
	}
}
func send(ctx context.Context, conn net.Conn, source io.Reader, size int64) (int64, error) {
	buffer := make([]byte, 64<<10)
	var header [4]byte
	var count int64
	emptyReads := 0
	for count < size {
		if ctx.Err() != nil {
			return count, failure("SCAN_TRANSPORT_FAILED", true)
		}
		length := int64(len(buffer))
		if size-count < length {
			length = size - count
		}
		n, readErr := source.Read(buffer[:length])
		if n > 0 {
			emptyReads = 0
			binary.BigEndian.PutUint32(header[:], uint32(n))
			if err := writeAll(conn, header[:]); err != nil {
				return count, err
			}
			if err := writeAll(conn, buffer[:n]); err != nil {
				return count, err
			}
			count += int64(n)
		} else {
			emptyReads++
		}
		if readErr != nil {
			if readErr == io.EOF && count == size {
				break
			}
			if readErr == io.EOF {
				return count, failure("SCAN_SOURCE_SIZE_MISMATCH", false)
			}
			return count, failure("SCAN_SOURCE_READ_FAILED", true)
		}
		if emptyReads > 100 {
			return count, failure("SCAN_SOURCE_READ_FAILED", true)
		}
	}
	// Refuse a longer source before sending the terminating success frame.
	for n := 0; n <= 100; n++ {
		amount, readErr := source.Read(buffer[:1])
		if amount > 0 {
			return count, failure("SCAN_SOURCE_SIZE_MISMATCH", false)
		}
		if readErr == io.EOF {
			binary.BigEndian.PutUint32(header[:], 0)
			return count, writeAll(conn, header[:])
		}
		if readErr != nil {
			return count, failure("SCAN_SOURCE_READ_FAILED", true)
		}
		if ctx.Err() != nil {
			return count, failure("SCAN_TRANSPORT_FAILED", true)
		}
	}
	return count, failure("SCAN_SOURCE_READ_FAILED", true)
}
