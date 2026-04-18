package binance

import (
	"bytes"
	"encoding/json"
)

// jsonDecoderStrict makes a Decoder that keeps default behavior (allows unknown fields).
// Kept as its own helper so account.go doesn't need to import bytes+json.
func jsonDecoderStrict(data []byte) *json.Decoder {
	return json.NewDecoder(bytes.NewReader(data))
}
