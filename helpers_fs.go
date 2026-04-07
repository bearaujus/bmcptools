package main

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// entryWithInfo pairs a directory entry with its cached FileInfo.
type entryWithInfo struct {
	entry os.DirEntry
	info  os.FileInfo
}

// collectFiles walks root and returns all matching file paths.
// Hidden files and directories (names starting with '.') are skipped unless showHidden is true.
func collectFiles(root string, recursive bool, globPattern string, showHidden bool) ([]string, error) {
	rootInfo, err := os.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("cannot stat %q: %w", root, err)
	}
	if !rootInfo.IsDir() {
		return []string{root}, nil
	}

	var files []string
	walkErr := filepath.WalkDir(root, func(p string, d os.DirEntry, werr error) error {
		if werr != nil {
			return nil
		}
		name := d.Name()
		if !showHidden && strings.HasPrefix(name, ".") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			if !recursive && p != root {
				return filepath.SkipDir
			}
			return nil
		}
		if globPattern != "" {
			matched := false
			for _, alt := range expandAlternation(globPattern) {
				if ok, _ := filepath.Match(alt, name); ok {
					matched = true
					break
				}
			}
			if !matched {
				return nil
			}
		}
		files = append(files, p)
		return nil
	})
	if walkErr != nil {
		return nil, fmt.Errorf("walk error: %w", walkErr)
	}
	return files, nil
}

// copyFileData copies src to dst with mode, discarding byte count.
func copyFileData(src, dst string, mode os.FileMode) error {
	_, err := copyFileDataN(src, dst, mode)
	return err
}

// copyFileDataN copies src to dst with mode and returns bytes copied.
func copyFileDataN(src, dst string, mode os.FileMode) (int64, error) {
	in, err := os.Open(src)
	if err != nil {
		return 0, fmt.Errorf("cannot open source: %w", err)
	}

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		in.Close()
		return 0, fmt.Errorf("cannot create destination: %w", err)
	}

	n, copyErr := io.Copy(out, in)
	in.Close()
	if closeErr := out.Close(); closeErr != nil && copyErr == nil {
		copyErr = closeErr
	}
	return n, copyErr
}

// countContentLines returns the number of lines in content.
// An empty string has 0 lines. A string with no trailing newline counts its last
// partial line.
func countContentLines(content string) int {
	if content == "" {
		return 0
	}
	n := strings.Count(content, "\n")
	if !strings.HasSuffix(content, "\n") {
		n++
	}
	return n
}

// applyReplaceToFile reads filePath, applies the replacement, and writes back.
// Returns (count, diffStr, skippedBinary, error).
func applyReplaceToFile(filePath, oldStr, newStr string, useRegex, dryRun, produceDiff bool) (int, string, bool, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return 0, "", false, err
	}
	// Skip binary files — check first 512 bytes only (same heuristic as sniffAndOpen).
	sniff := data
	if len(sniff) > 512 {
		sniff = data[:512]
	}
	if isBinaryContent(sniff, http.DetectContentType(sniff)) {
		return 0, "", true, nil
	}

	original, hasCRLF := normalizeCRLF(string(data))
	modified, count, editErr := applyEdit(original, oldStr, newStr, useRegex, true)
	if editErr != nil {
		return 0, "", false, editErr
	}
	if count == 0 {
		return 0, "", false, nil
	}

	diff := ""
	if produceDiff {
		diff = generateDiff(original, modified, 2)
	}

	if !dryRun {
		absPath, _ := filepath.Abs(filePath)
		unlock := lockFile(absPath)
		if hasCRLF {
			modified = restoreCRLF(modified)
		}
		wErr := atomicWriteFile(filePath, []byte(modified), 0o644)
		unlock()
		if wErr != nil {
			return count, diff, false, wErr
		}
	}
	return count, diff, false, nil
}

// fileLocks serializes concurrent writes to the same absolute file path,
// preventing silent data loss when two tool calls race on the same file.
var fileLocks sync.Map // map[string]*sync.Mutex

// lockFile returns the per-file mutex for the given absolute path and locks it.
// The caller must call the returned unlock function when done.
func lockFile(absPath string) func() {
	v, _ := fileLocks.LoadOrStore(absPath, &sync.Mutex{})
	mu := v.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}

// mkdirAllClear calls os.MkdirAll and, on failure, augments the error message
// to identify the specific path component that is blocking directory creation
// (e.g. an existing file occupying a directory slot).
func mkdirAllClear(dir string, perm os.FileMode) error {
	if err := os.MkdirAll(dir, perm); err == nil {
		return nil
	}
	// Walk path segments to find the blocking component.
	parts := strings.Split(filepath.ToSlash(filepath.Clean(dir)), "/")
	cur := ""
	if filepath.IsAbs(dir) {
		cur = "/"
	}
	for _, part := range parts {
		if part == "" {
			continue
		}
		cur = filepath.Join(cur, part)
		info, statErr := os.Stat(cur)
		if statErr != nil {
			break
		}
		if !info.IsDir() {
			return fmt.Errorf("%q already exists as a file; cannot create a directory there", cur)
		}
	}
	// Fall back to re-running MkdirAll to surface the original OS error.
	return os.MkdirAll(dir, perm)
}

// atomicWriteFile writes content to path atomically: it creates a temp file in
// the same directory, writes the content, then renames it to path. This ensures
// that a crash or interrupt never leaves the target file in a partial state.
func atomicWriteFile(path string, content []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".bmcptools-write-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()

	if _, err := tmp.Write(content); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return os.Rename(tmpPath, path)
}

// linesByScanner reads a file and returns all lines as a slice.
func linesByScanner(f *os.File) ([]string, error) {
	scanner := bufio.NewScanner(f)
	scanBuf := make([]byte, 64*1024)
	scanner.Buffer(scanBuf, 10*1024*1024)
	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	return lines, scanner.Err()
}
