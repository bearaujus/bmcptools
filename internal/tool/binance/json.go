package binance

import (
	"bytes"
	"encoding/json"
	"strconv"
)

// jsonDecoderStrict makes a Decoder that keeps default behavior (allows unknown fields).
// Kept as its own helper so account.go doesn't need to import bytes+json.
func jsonDecoderStrict(data []byte) *json.Decoder {
	return json.NewDecoder(bytes.NewReader(data))
}

// jsonDecoderUseNumber returns a Decoder with UseNumber enabled, so numeric fields
// are kept as json.Number instead of being converted to float64. This prevents
// precision loss for large integer IDs (e.g., algoId).
func jsonDecoderUseNumber(data []byte) *json.Decoder {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	return dec
}

// algoIDStr extracts an algo order ID string from a map value. Supports both
// json.Number (from UseNumber decoder) and legacy float64.
func algoIDStr(v interface{}) string {
	switch n := v.(type) {
	case json.Number:
		return string(n)
	case float64:
		return strconv.FormatInt(int64(n), 10)
	default:
		return ""
	}
}
