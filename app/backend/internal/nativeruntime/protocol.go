package nativeruntime

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"io"

	"copypaste.hk/platform/internal/agentworker"
)

type Request struct {
	Action    string                      `json:"action"`
	Node      string                      `json:"node,omitempty"`
	Entry     string                      `json:"entry,omitempty"`
	Input     *WorkerInput                `json:"input,omitempty"`
	Challenge *agentworker.ProofChallenge `json:"challenge,omitempty"`
}
type Reply struct {
	Code    string                  `json:"code"`
	Stopped bool                    `json:"stopped,omitempty"`
	Proof   *agentworker.ProofReply `json:"proof,omitempty"`
}

// A length frame lets the launcher keep its write side open. Its disappearance
// then cancels the owned unit instead of being confused with request completion.
func WriteFrame(w io.Writer, v any) error {
	raw, err := json.Marshal(v)
	if err != nil || len(raw) > MaxInput {
		return invalid()
	}
	var length [4]byte
	binary.BigEndian.PutUint32(length[:], uint32(len(raw)))
	if _, err = io.Copy(w, bytes.NewReader(append(length[:], raw...))); err != nil {
		return invalid()
	}
	return nil
}
func ReadFrame(r io.Reader, v any) error {
	var length [4]byte
	if _, err := io.ReadFull(r, length[:]); err != nil {
		return invalid()
	}
	size := binary.BigEndian.Uint32(length[:])
	if size == 0 || size > MaxInput {
		return invalid()
	}
	raw := make([]byte, size)
	if _, err := io.ReadFull(r, raw); err != nil {
		return invalid()
	}
	return Decode(bytes.NewReader(raw), v, MaxInput)
}
