package filescan

import (
	"os"
	"strconv"
	"time"
)

func Load() (Scanner, error) {
	driver := os.Getenv("CP_SCAN_DRIVER")
	if driver == "" || driver == "disabled" {
		return Disabled{}, nil
	}
	if driver != "clamav" {
		return nil, failure("SCAN_CONFIGURATION_INVALID", false)
	}
	maximum, err := strconv.ParseInt(os.Getenv("CP_SCAN_MAX_BYTES"), 10, 64)
	if err != nil {
		return nil, failure("SCAN_CONFIGURATION_INVALID", false)
	}
	seconds, err := strconv.Atoi(os.Getenv("CP_SCAN_TIMEOUT_SECONDS"))
	if err != nil || seconds < 1 || seconds > 900 {
		return nil, failure("SCAN_CONFIGURATION_INVALID", false)
	}
	return New(Config{Network: os.Getenv("CP_CLAMD_NETWORK"), Address: os.Getenv("CP_CLAMD_ADDRESS"), Policy: Policy{
		MaxBytes: maximum, Timeout: time.Duration(seconds) * time.Second, SHA256: os.Getenv("CP_SCAN_POLICY_SHA256"), LimitsAttested: os.Getenv("CP_SCAN_LIMITS_ATTESTED") == "true"}})
}
