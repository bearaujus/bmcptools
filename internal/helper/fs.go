package helper

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

// EntryWithInfo pairs a directory entry with its cached FileInfo.
type EntryWithInfo struct {
	Entry os.DirEntry
	Info  os.FileInfo
}

// CollectFiles walks root and returns all matching file paths.
// Optional exclude patterns match entry basenames and prune matching directories.
func CollectFiles(root string, recursive bool, globPattern string, showHidden bool, excludePatternSets ...[]string) ([]string, error) {
	rootInfo, err := os.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("cannot stat %q: %w", root, err)
	}
	if !rootInfo.IsDir() {
		return []string{root}, nil
	}

	var excludePatterns []string
	if len(excludePatternSets) > 0 {
		excludePatterns = excludePatternSets[0]
	}

	var files []string
	walkErr := filepath.WalkDir(root, func(p string, d os.DirEntry, werr error) error {
		if werr != nil {
			return nil
		}
		name := d.Name()
		if p != root && MatchesAnyGlobName(name, excludePatterns) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
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
			for _, alt := range ExpandAlternation(globPattern) {
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

// CopyFileData copies src to dst with mode, discarding byte count.
func CopyFileData(src, dst string, mode os.FileMode) error {
	_, err := CopyFileDataN(src, dst, mode)
	return err
}

// CopyFileDataN copies src to dst with mode and returns bytes copied.
func CopyFileDataN(src, dst string, mode os.FileMode) (int64, error) {
	in, err := os.Open(src)
	if err != nil {
		return 0, fmt.Errorf("cannot open source: %w", err)
	}
	defer in.Close()

	dir := filepath.Dir(dst)
	tmp, err := os.CreateTemp(dir, ".bmcptools-copy-*")
	if err != nil {
		return 0, fmt.Errorf("cannot create destination: %w", err)
	}
	tmpPath := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()

	n, copyErr := io.Copy(tmp, in)
	if copyErr == nil {
		copyErr = tmp.Chmod(mode)
	}
	if closeErr := tmp.Close(); closeErr != nil && copyErr == nil {
		copyErr = closeErr
	}
	if copyErr != nil {
		return n, copyErr
	}
	if err := os.Rename(tmpPath, dst); err != nil {
		return n, err
	}
	cleanup = false
	return n, nil
}

// CountContentLines returns the number of lines in content.
func CountContentLines(content string) int {
	if content == "" {
		return 0
	}
	n := strings.Count(content, "\n")
	if !strings.HasSuffix(content, "\n") {
		n++
	}
	return n
}

// ExistingFilePerm returns path's current permission bits when it exists.
func ExistingFilePerm(path string, fallback os.FileMode) os.FileMode {
	info, err := os.Stat(path)
	if err != nil {
		return fallback
	}
	return info.Mode().Perm()
}

// ApplyReplaceToFile reads filePath, applies the replacement, and writes back.
// Return order: (matchCount int, diffStr string, skippedBinary bool, err error).
// When dryRun=true the file is never modified; diff is still produced if produceDiff=true.
// When the file contains binary content, skippedBinary=true and all other values are zero.
func ApplyReplaceToFile(filePath, oldStr, newStr string, useRegex, dryRun, produceDiff bool) (int, string, bool, error) {
	if !dryRun {
		absPath, _ := filepath.Abs(filePath)
		unlock := LockFile(absPath)
		defer unlock()
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return 0, "", false, err
	}
	sniff := data
	if len(sniff) > 512 {
		sniff = data[:512]
	}
	if IsBinaryContent(sniff, http.DetectContentType(sniff)) {
		return 0, "", true, nil
	}

	original, hasCRLF := NormalizeCRLF(string(data))
	modified, count, editErr := ApplyEdit(original, oldStr, newStr, useRegex, true)
	if editErr != nil {
		return 0, "", false, editErr
	}
	if count == 0 {
		return 0, "", false, nil
	}

	diff := ""
	if produceDiff {
		diff = GenerateDiff(original, modified, 2)
	}

	if !dryRun {
		if hasCRLF {
			modified = RestoreCRLF(modified)
		}
		wErr := AtomicWriteFile(filePath, []byte(modified), ExistingFilePerm(filePath, 0o644))
		if wErr != nil {
			return count, diff, false, wErr
		}
	}
	return count, diff, false, nil
}

var fileLocks sync.Map

// LockFile returns the per-file mutex for the given absolute path and locks it.
// The caller must call the returned unlock function when done.
func LockFile(absPath string) func() {
	v, _ := fileLocks.LoadOrStore(absPath, &sync.Mutex{})
	mu := v.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}

// MkdirAllClear calls os.MkdirAll and augments the error message on failure.
// It walks every path ancestor (root → leaf) using filepath.Dir, so the
// detection works correctly on both Unix and Windows (including drive letters).
func MkdirAllClear(dir string, perm os.FileMode) error {
	if err := os.MkdirAll(dir, perm); err == nil {
		return nil
	}
	// Collect ancestors leaf→root, then reverse to walk root→leaf.
	cleanDir := filepath.Clean(dir)
	var ancestors []string
	for p := cleanDir; ; p = filepath.Dir(p) {
		ancestors = append(ancestors, p)
		if parent := filepath.Dir(p); parent == p {
			break
		}
	}
	for i, j := 0, len(ancestors)-1; i < j; i, j = i+1, j-1 {
		ancestors[i], ancestors[j] = ancestors[j], ancestors[i]
	}
	for _, cur := range ancestors {
		info, statErr := os.Stat(cur)
		if statErr != nil {
			break // path no longer exists; stop scanning
		}
		if !info.IsDir() {
			return fmt.Errorf("%q already exists as a file; cannot create a directory there", cur)
		}
	}
	return os.MkdirAll(dir, perm)
}

// AtomicWriteFile writes content to path atomically via a temp file + rename.
func AtomicWriteFile(path string, content []byte, perm os.FileMode) error {
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

// LinesByScanner reads a file and returns all lines as a slice.
func LinesByScanner(f *os.File) ([]string, error) {
	scanner := bufio.NewScanner(f)
	scanBuf := make([]byte, 64*1024)
	scanner.Buffer(scanBuf, 10*1024*1024)
	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	return lines, scanner.Err()
}
