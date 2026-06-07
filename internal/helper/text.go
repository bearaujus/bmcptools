package helper

import (
	"bufio"
	"encoding/binary"
	"io"
	"net/http"
	"os"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

// TextEncoding identifies the on-disk encoding used for a decoded text file.
type TextEncoding string

const (
	TextEncodingUTF8    TextEncoding = "utf-8"
	TextEncodingUTF16LE TextEncoding = "utf-16le"
	TextEncodingUTF16BE TextEncoding = "utf-16be"
)

// DecodedText holds normalized text plus enough metadata to write it back.
type DecodedText struct {
	Text     string
	Encoding TextEncoding
	HadBOM   bool
	HasCRLF  bool
}

// DetectTextEncoding returns an explicit BOM-marked encoding when present.
func DetectTextEncoding(header []byte) (TextEncoding, bool) {
	if len(header) >= 3 && header[0] == 0xEF && header[1] == 0xBB && header[2] == 0xBF {
		return TextEncodingUTF8, true
	}
	if len(header) >= 2 && header[0] == 0xFF && header[1] == 0xFE {
		return TextEncodingUTF16LE, true
	}
	if len(header) >= 2 && header[0] == 0xFE && header[1] == 0xFF {
		return TextEncodingUTF16BE, true
	}
	return TextEncodingUTF8, false
}

// DecodeTextBytes decodes BOM-marked UTF-16 and normalizes CRLF endings.
func DecodeTextBytes(data []byte) DecodedText {
	encoding, hadBOM := DetectTextEncoding(data)
	decoded := DecodedText{
		Encoding: encoding,
		HadBOM:   hadBOM,
	}

	var text string
	switch encoding {
	case TextEncodingUTF16LE, TextEncodingUTF16BE:
		payload := data
		if hadBOM && len(payload) >= 2 {
			payload = payload[2:]
		}
		text = decodeUTF16(payload, encoding == TextEncodingUTF16LE)
	default:
		payload := data
		if hadBOM && len(payload) >= 3 {
			payload = payload[3:]
		}
		text = string(payload)
		if !utf8.Valid(payload) {
			text = strings.ToValidUTF8(text, "\uFFFD")
		}
	}
	decoded.Text, decoded.HasCRLF = NormalizeCRLF(text)
	return decoded
}

// EncodeTextBytes re-encodes normalized text using the original encoding.
func EncodeTextBytes(text string, encoding TextEncoding, includeBOM bool) []byte {
	switch encoding {
	case TextEncodingUTF16LE:
		return encodeUTF16(text, binary.LittleEndian, includeBOM)
	case TextEncodingUTF16BE:
		return encodeUTF16(text, binary.BigEndian, includeBOM)
	default:
		out := []byte(text)
		if includeBOM {
			out = append([]byte{0xEF, 0xBB, 0xBF}, out...)
		}
		return out
	}
}

// OpenTextScanner opens a scanner for text content, decoding BOM-marked UTF-16
// eagerly and leaving UTF-8 files streamed from disk.
func OpenTextScanner(path string, maxTokenBytes int) (*bufio.Scanner, func() error, bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, false, err
	}
	header, err := readSniffHeader(f)
	if err != nil {
		_ = f.Close()
		return nil, nil, false, err
	}
	contentType := http.DetectContentType(header)
	if IsBinaryContent(header, contentType) {
		_ = f.Close()
		return nil, nil, true, nil
	}
	if encoding, hadBOM := DetectTextEncoding(header); hadBOM && encoding != TextEncodingUTF8 {
		data, err := io.ReadAll(f)
		_ = f.Close()
		if err != nil {
			return nil, nil, false, err
		}
		decoded := DecodeTextBytes(data)
		scanner := bufio.NewScanner(strings.NewReader(decoded.Text))
		scanner.Buffer(make([]byte, 64*1024), maxTokenBytes)
		return scanner, func() error { return nil }, false, nil
	}
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), maxTokenBytes)
	return scanner, f.Close, false, nil
}

func decodeUTF16(data []byte, littleEndian bool) string {
	units := make([]uint16, 0, len(data)/2)
	var order binary.ByteOrder = binary.BigEndian
	if littleEndian {
		order = binary.LittleEndian
	}
	for i := 0; i+1 < len(data); i += 2 {
		units = append(units, order.Uint16(data[i:i+2]))
	}
	text := string(utf16.Decode(units))
	if len(data)%2 != 0 {
		text += "\uFFFD"
	}
	return text
}

func encodeUTF16(text string, order binary.ByteOrder, includeBOM bool) []byte {
	runes := utf16.Encode([]rune(text))
	extra := 0
	if includeBOM {
		extra = 2
	}
	out := make([]byte, extra+len(runes)*2)
	offset := 0
	if includeBOM {
		if order == binary.LittleEndian {
			out[0], out[1] = 0xFF, 0xFE
		} else {
			out[0], out[1] = 0xFE, 0xFF
		}
		offset = 2
	}
	for i, unit := range runes {
		order.PutUint16(out[offset+i*2:offset+i*2+2], unit)
	}
	return out
}
