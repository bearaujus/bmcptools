package helper

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"os"
)

// DefaultMaxReadBytes is the default limit for reading a single file as text.
// Keep this conservative: tool output goes directly into model context.
const DefaultMaxReadBytes = 128 * 1024

// AutoLineCountMaxBytes is the largest file size for which line counts are
// computed automatically when the caller did not explicitly force counting.
const AutoLineCountMaxBytes int64 = 1 * 1024 * 1024

// ReadBinaryFile returns a compact binary file summary, optionally including
// capped base64 content when explicitly requested.
func ReadBinaryFile(f *os.File, info os.FileInfo, contentType string, limit int, includeBase64 bool) (string, error) {
	if !includeBase64 {
		return fmt.Sprintf(
			"[BINARY FILE] %s: %s, content-type %s. Base64 omitted; set include_base64=true to return capped encoded bytes.",
			info.Name(), HumanizeBytes(info.Size()), contentType,
		), nil
	}

	readN := limit
	if int(info.Size()) < readN {
		readN = int(info.Size())
	}
	data := make([]byte, readN)
	n, err := io.ReadFull(f, data)
	if err != nil && err != io.ErrUnexpectedEOF {
		return "", fmt.Errorf("read error: %w", err)
	}
	data = data[:n]

	encoded := base64.StdEncoding.EncodeToString(data)
	msg := fmt.Sprintf(
		"[BINARY FILE]\nPath: %s\nSize: %s\nContent-Type: %s\n\nBase64:\n%s",
		info.Name(), HumanizeBytes(info.Size()), contentType, encoded,
	)
	if int(info.Size()) > limit {
		msg += fmt.Sprintf(
			"\n\n[Truncated: first %s of %s shown]",
			HumanizeBytes(int64(limit)), HumanizeBytes(info.Size()),
		)
	}
	return msg, nil
}

// ReadFullText reads the entire file as text up to limit bytes.
// Returns (text, truncated, renderedLineCount, error).
func ReadFullText(f *os.File, info os.FileInfo, limit int) (string, bool, int, error) {
	limited := io.LimitReader(f, int64(limit)+1)
	raw, err := io.ReadAll(limited)
	if err != nil {
		return "", false, 0, fmt.Errorf("read error: %w", err)
	}

	truncated := len(raw) > limit
	if truncated {
		raw = raw[:limit]
	}

	decoded := DecodeTextBytes(raw)
	lineCount := CountContentLines(decoded.Text)
	result := decoded.Text
	if truncated {
		result += fmt.Sprintf(
			"\n\n[TRUNCATED — showing first %s of %s. Use start_line/end_line to read specific sections.]",
			HumanizeBytes(int64(limit)), HumanizeBytes(info.Size()),
		)
	}
	return result, truncated, lineCount, nil
}

// SniffAndOpen opens the file at p, sniffs binary content, then returns an
// open file (seeked back to 0), the file info, contentType, and isBinary.
// The caller is responsible for closing the file.
func SniffAndOpen(p string) (f *os.File, info os.FileInfo, contentType string, binary bool, err error) {
	f, err = os.Open(p)
	if err != nil {
		return nil, nil, "", false, fmt.Errorf("cannot open: %w", err)
	}

	info, err = f.Stat()
	if err != nil {
		f.Close()
		return nil, nil, "", false, fmt.Errorf("cannot stat: %w", err)
	}
	if info.IsDir() {
		f.Close()
		return nil, nil, "", false, fmt.Errorf("path is a directory; use list_directory instead")
	}

	header, err := readSniffHeader(f)
	if err != nil {
		f.Close()
		return nil, nil, "", false, err
	}

	contentType = http.DetectContentType(header)
	binary = IsBinaryContent(header, contentType)
	return f, info, contentType, binary, nil
}

// ReadOneFileAsText reads a single file as text, returning a compact binary
// summary for binary files unless includeBase64 is true.
func ReadOneFileAsText(p string, limitBytes int, includeBase64 bool) (string, error) {
	f, info, contentType, binary, err := SniffAndOpen(p)
	if err != nil {
		return "", err
	}
	defer f.Close()

	if binary {
		return ReadBinaryFile(f, info, contentType, limitBytes, includeBase64)
	}
	text, _, _, err := ReadFullText(f, info, limitBytes)
	return text, err
}

// CountTextFileLines counts lines for text files. When force is false, large
// files are skipped to keep metadata-style calls cheap.
func CountTextFileLines(path string, force bool) (count int, counted bool, skippedForSize bool, err error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, false, false, err
	}
	return CountTextFileLinesWithInfo(path, info, force)
}

// CountTextFileLinesWithInfo counts lines for text files using already-known metadata.
func CountTextFileLinesWithInfo(path string, info os.FileInfo, force bool) (count int, counted bool, skippedForSize bool, err error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, false, false, err
	}
	defer f.Close()

	if !force && info.Size() > AutoLineCountMaxBytes {
		return 0, false, true, nil
	}

	header, err := readSniffHeader(f)
	if err != nil {
		return 0, false, false, err
	}
	if IsBinaryContent(header, http.DetectContentType(header)) {
		return 0, false, false, nil
	}
	if encoding, hasBOM := DetectTextEncoding(header); hasBOM && encoding != TextEncodingUTF8 {
		data, err := io.ReadAll(f)
		if err != nil {
			return 0, false, false, err
		}
		return CountContentLines(DecodeTextBytes(data).Text), true, false, nil
	}
	n, err := CountLines(f)
	if err != nil {
		return 0, false, false, err
	}
	return n, true, false, nil
}

// CountLines counts the total number of lines in an open file.
func CountLines(f *os.File) (int, error) {
	buf := make([]byte, 64*1024)
	count := 0
	sawAny := false
	lastEndedWithNewline := false
	for {
		n, err := f.Read(buf)
		if n > 0 {
			sawAny = true
			count += bytes.Count(buf[:n], []byte{'\n'})
			lastEndedWithNewline = buf[n-1] == '\n'
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return 0, err
		}
	}
	if sawAny && !lastEndedWithNewline {
		count++
	}
	return count, nil
}

// NormalizeTextBytes strips a UTF-8 BOM, replaces invalid UTF-8 only when
// needed, and normalizes CRLF line endings.
func NormalizeTextBytes(data []byte) (string, bool) {
	decoded := DecodeTextBytes(data)
	return decoded.Text, decoded.HasCRLF
}

func countLineBytes(data []byte) int {
	if len(data) == 0 {
		return 0
	}
	count := bytes.Count(data, []byte{'\n'})
	if data[len(data)-1] != '\n' {
		count++
	}
	return count
}

func readSniffHeader(f *os.File) ([]byte, error) {
	header := make([]byte, 512)
	n, err := io.ReadFull(f, header)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return nil, fmt.Errorf("read error: %w", err)
	}
	header = header[:n]
	if _, seekErr := f.Seek(0, io.SeekStart); seekErr != nil {
		return nil, fmt.Errorf("seek error: %w", seekErr)
	}
	return header, nil
}
