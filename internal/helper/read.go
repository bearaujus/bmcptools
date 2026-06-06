package helper

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"unicode/utf8"
)

// DefaultMaxReadBytes is the default limit for reading a single file as text.
// Keep this conservative: tool output goes directly into model context.
const DefaultMaxReadBytes = 256 * 1024

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
// Returns (text, truncated, error).
func ReadFullText(f *os.File, info os.FileInfo, limit int) (string, bool, error) {
	limited := io.LimitReader(f, int64(limit)+1)
	raw, err := io.ReadAll(limited)
	if err != nil {
		return "", false, fmt.Errorf("read error: %w", err)
	}

	truncated := len(raw) > limit
	if truncated {
		raw = raw[:limit]
	}

	raw = StripBOM(raw)
	if !utf8.Valid(raw) {
		raw = []byte(strings.ToValidUTF8(string(raw), "\uFFFD"))
	}

	result := string(raw)
	if truncated {
		if _, seekErr := f.Seek(0, io.SeekStart); seekErr == nil {
			scanner := bufio.NewScanner(f)
			scanBuf := make([]byte, 64*1024)
			scanner.Buffer(scanBuf, 10*1024*1024)
			totalLines := 0
			for scanner.Scan() {
				totalLines++
			}
			result += fmt.Sprintf(
				"\n\n[TRUNCATED — showing first %s of %s (%s total). Use start_line/end_line to read specific sections.]",
				HumanizeBytes(int64(limit)), HumanizeBytes(info.Size()), Pluralize(totalLines, "line"),
			)
		} else {
			result += fmt.Sprintf(
				"\n\n[TRUNCATED — showing first %s of %s. Use start_line/end_line to read specific sections.]",
				HumanizeBytes(int64(limit)), HumanizeBytes(info.Size()),
			)
		}
	}
	return result, truncated, nil
}

// SniffAndOpen opens the file at p, sniffs binary content, then returns an
// open file (seeked back to 0), the file info, contentType, and isBinary.
// The caller is responsible for closing the file.
func SniffAndOpen(p string) (f *os.File, info os.FileInfo, contentType string, binary bool, err error) {
	info, err = os.Stat(p)
	if err != nil {
		return nil, nil, "", false, fmt.Errorf("cannot stat: %w", err)
	}
	if info.IsDir() {
		return nil, nil, "", false, fmt.Errorf("path is a directory; use list_directory instead")
	}

	f, err = os.Open(p)
	if err != nil {
		return nil, nil, "", false, fmt.Errorf("cannot open: %w", err)
	}

	header := make([]byte, 512)
	n, _ := f.Read(header)
	header = header[:n]
	if _, seekErr := f.Seek(0, io.SeekStart); seekErr != nil {
		f.Close()
		return nil, nil, "", false, fmt.Errorf("seek error: %w", seekErr)
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
	text, _, err := ReadFullText(f, info, limitBytes)
	return text, err
}

// CountTextFileLines counts lines for text files. When force is false, large
// files are skipped to keep metadata-style calls cheap.
func CountTextFileLines(path string, force bool) (count int, counted bool, skippedForSize bool, err error) {
	f, info, _, binary, err := SniffAndOpen(path)
	if err != nil {
		return 0, false, false, err
	}
	defer f.Close()

	if !force && info.Size() > AutoLineCountMaxBytes {
		return 0, false, true, nil
	}
	if binary {
		return 0, false, false, nil
	}
	n, err := CountLines(f)
	if err != nil {
		return 0, false, false, err
	}
	return n, true, false, nil
}

// CountLines counts the total number of lines in an open file.
func CountLines(f *os.File) (int, error) {
	scanner := bufio.NewScanner(f)
	scanBuf := make([]byte, 64*1024)
	scanner.Buffer(scanBuf, 10*1024*1024)
	count := 0
	for scanner.Scan() {
		count++
	}
	return count, scanner.Err()
}
