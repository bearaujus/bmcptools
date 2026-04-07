package main

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

// readBinaryFile reads a binary file and returns a base64-encoded text summary.
func readBinaryFile(f *os.File, info os.FileInfo, contentType string, limit int) (string, error) {
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
		info.Name(), humanizeBytes(info.Size()), contentType, encoded,
	)
	if int(info.Size()) > limit {
		msg += fmt.Sprintf(
			"\n\n[Truncated: first %s of %s shown]",
			humanizeBytes(int64(limit)), humanizeBytes(info.Size()),
		)
	}
	return msg, nil
}

// readFullText reads the entire file as text up to limit bytes.
// Returns (text, truncated, error). When truncated=true, the text includes a
// notice with total line count for navigation.
func readFullText(f *os.File, info os.FileInfo, limit int) (string, bool, error) {
	limited := io.LimitReader(f, int64(limit)+1)
	raw, err := io.ReadAll(limited)
	if err != nil {
		return "", false, fmt.Errorf("read error: %w", err)
	}

	truncated := len(raw) > limit
	if truncated {
		raw = raw[:limit]
	}

	raw = stripBOM(raw)
	if !utf8.Valid(raw) {
		raw = []byte(strings.ToValidUTF8(string(raw), "\uFFFD"))
	}

	result := string(raw)
	if truncated {
		// Count total lines so the caller knows the file extent for navigation.
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
				humanizeBytes(int64(limit)), humanizeBytes(info.Size()), pluralize(totalLines, "line"),
			)
		} else {
			result += fmt.Sprintf(
				"\n\n[TRUNCATED — showing first %s of %s. Use start_line/end_line to read specific sections.]",
				humanizeBytes(int64(limit)), humanizeBytes(info.Size()),
			)
		}
	}
	return result, truncated, nil
}

// sniffAndOpen opens the file at p, sniffs binary content, then returns an
// open file (seeked back to 0), the file info, contentType, and isBinary.
// The caller is responsible for closing the file.
func sniffAndOpen(p string) (f *os.File, info os.FileInfo, contentType string, binary bool, err error) {
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
	binary = isBinaryContent(header, contentType)
	return f, info, contentType, binary, nil
}

// readOneFileAsText reads a single file as text, returning a base64 summary for binary files.
func readOneFileAsText(p string, limitBytes int) (string, error) {
	f, info, contentType, binary, err := sniffAndOpen(p)
	if err != nil {
		return "", err
	}
	defer f.Close()

	if binary {
		return readBinaryFile(f, info, contentType, limitBytes)
	}
	text, _, err := readFullText(f, info, limitBytes)
	return text, err
}

// countLines counts the total number of lines in an open file.
func countLines(f *os.File) (int, error) {
	scanner := bufio.NewScanner(f)
	scanBuf := make([]byte, 64*1024)
	scanner.Buffer(scanBuf, 10*1024*1024)
	count := 0
	for scanner.Scan() {
		count++
	}
	return count, scanner.Err()
}
