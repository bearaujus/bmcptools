package file

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/bearaujus/bmcptools/internal/helper"
)

type DeleteStats struct {
	Kind string
	Size int64
}

type CopyStats struct {
	SourceKind string
	Files      int
	Dirs       int
	Bytes      int64
}

type MoveStats struct {
	SourceKind   string
	Files        int
	Dirs         int
	Bytes        int64
	UsedFallback bool
}

func DeleteEntry(path string) (DeleteStats, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return DeleteStats{}, fmt.Errorf("cannot stat %q: %w", path, err)
	}
	if info.IsDir() {
		return DeleteStats{}, fmt.Errorf("path is a directory; use delete_directory instead")
	}
	kind := "file"
	if info.Mode()&os.ModeSymlink != 0 {
		kind = "symlink"
	}
	if err := os.Remove(path); err != nil {
		return DeleteStats{}, fmt.Errorf("cannot delete %q: %w", path, err)
	}
	return DeleteStats{Kind: kind, Size: info.Size()}, nil
}

func CopyPath(source, destination string, overwrite bool) (CopyStats, error) {
	srcInfo, err := os.Stat(source)
	if err != nil {
		return CopyStats{}, fmt.Errorf("cannot stat source: %w", err)
	}
	if srcInfo.IsDir() {
		return copyDirectory(source, destination, overwrite, srcInfo)
	}
	return copyRegularFile(source, destination, overwrite, srcInfo)
}

func MovePath(source, destination string, overwrite bool) (MoveStats, error) {
	srcInfo, err := os.Stat(source)
	if err != nil {
		return MoveStats{}, fmt.Errorf("cannot stat source: %w", err)
	}

	destinationExists := false
	var dstInfo os.FileInfo
	if info, statErr := os.Stat(destination); statErr == nil {
		destinationExists = true
		dstInfo = info
		if os.SameFile(srcInfo, dstInfo) {
			return MoveStats{}, fmt.Errorf("source and destination refer to the same file; no move performed")
		}
		if !overwrite {
			return MoveStats{}, fmt.Errorf("destination %q already exists; set overwrite=true to replace it", destination)
		}
	} else if !os.IsNotExist(statErr) {
		return MoveStats{}, fmt.Errorf("cannot stat destination: %w", statErr)
	}

	if srcInfo.IsDir() {
		if destinationExists && !dstInfo.IsDir() {
			return MoveStats{}, fmt.Errorf("destination is a file; directory moves require a directory destination")
		}
		if err := helper.MkdirAllClear(filepath.Dir(destination), 0o755); err != nil {
			return MoveStats{}, fmt.Errorf("cannot create destination directory: %w", err)
		}
		if !destinationExists {
			if err := os.Rename(source, destination); err == nil {
				return MoveStats{SourceKind: "directory", Dirs: 1}, nil
			}
		}

		copyStats, err := CopyPath(source, destination, true)
		if err != nil {
			return MoveStats{}, fmt.Errorf("copy error during directory move: %w", err)
		}
		if err := os.RemoveAll(source); err != nil {
			return MoveStats{}, fmt.Errorf("directory copied to %s but could not delete source %s: %v", destination, source, err)
		}
		return MoveStats{
			SourceKind:   "directory",
			Files:        copyStats.Files,
			Dirs:         copyStats.Dirs,
			Bytes:        copyStats.Bytes,
			UsedFallback: true,
		}, nil
	}

	if destinationExists {
		if dstInfo.IsDir() {
			return MoveStats{}, fmt.Errorf("destination is a directory; file moves require a file path")
		}
		if err := helper.MkdirAllClear(filepath.Dir(destination), 0o755); err != nil {
			return MoveStats{}, fmt.Errorf("cannot create destination directory: %w", err)
		}
		if err := helper.CopyFileData(source, destination, srcInfo.Mode().Perm()); err != nil {
			return MoveStats{}, fmt.Errorf("copy error during move: %w", err)
		}
		if err := os.Remove(source); err != nil {
			return MoveStats{}, fmt.Errorf("file copied to %s but could not delete source %s: %v", destination, source, err)
		}
		return MoveStats{
			SourceKind:   "file",
			Files:        1,
			Bytes:        srcInfo.Size(),
			UsedFallback: true,
		}, nil
	}

	if err := helper.MkdirAllClear(filepath.Dir(destination), 0o755); err != nil {
		return MoveStats{}, fmt.Errorf("cannot create destination directory: %w", err)
	}
	if err := os.Rename(source, destination); err == nil {
		return MoveStats{SourceKind: "file", Files: 1, Bytes: srcInfo.Size()}, nil
	}
	if err := helper.CopyFileData(source, destination, srcInfo.Mode().Perm()); err != nil {
		return MoveStats{}, fmt.Errorf("copy error during move: %w", err)
	}
	if err := os.Remove(source); err != nil {
		return MoveStats{}, fmt.Errorf("file copied to %s but could not delete source %s: %v", destination, source, err)
	}
	return MoveStats{
		SourceKind:   "file",
		Files:        1,
		Bytes:        srcInfo.Size(),
		UsedFallback: true,
	}, nil
}

func copyRegularFile(source, destination string, overwrite bool, srcInfo os.FileInfo) (CopyStats, error) {
	if dstInfo, err := os.Stat(destination); err == nil {
		if os.SameFile(srcInfo, dstInfo) {
			return CopyStats{}, fmt.Errorf("source and destination refer to the same file; refusing to copy")
		}
		if dstInfo.IsDir() {
			return CopyStats{}, fmt.Errorf("destination is a directory; copy_file requires a file path when source is a file")
		}
		if !overwrite {
			return CopyStats{}, fmt.Errorf("destination %q already exists; set overwrite=true to replace it", destination)
		}
	} else if !os.IsNotExist(err) {
		return CopyStats{}, fmt.Errorf("cannot stat destination: %w", err)
	}

	if err := helper.MkdirAllClear(filepath.Dir(destination), 0o755); err != nil {
		return CopyStats{}, fmt.Errorf("cannot create destination directory: %w", err)
	}
	n, err := helper.CopyFileDataN(source, destination, srcInfo.Mode().Perm())
	if err != nil {
		return CopyStats{}, fmt.Errorf("copy error: %w", err)
	}
	return CopyStats{SourceKind: "file", Files: 1, Bytes: n}, nil
}

func copyDirectory(source, destination string, overwrite bool, srcInfo os.FileInfo) (CopyStats, error) {
	if linfo, err := os.Lstat(source); err == nil && linfo.Mode()&os.ModeSymlink != 0 {
		return CopyStats{}, fmt.Errorf("copying a directory symlink is not supported")
	}

	sourceAbs, err := filepath.Abs(source)
	if err != nil {
		return CopyStats{}, fmt.Errorf("invalid source path: %w", err)
	}
	destinationAbs, err := filepath.Abs(destination)
	if err != nil {
		return CopyStats{}, fmt.Errorf("invalid destination path: %w", err)
	}
	if sameCleanPath(sourceAbs, destinationAbs) || isSubpath(sourceAbs, destinationAbs) {
		return CopyStats{}, fmt.Errorf("destination %q must not be inside source directory %q", destination, source)
	}

	if dstInfo, err := os.Stat(destination); err == nil {
		if !dstInfo.IsDir() {
			return CopyStats{}, fmt.Errorf("destination is a file; directory copies require a directory destination")
		}
		if !overwrite {
			return CopyStats{}, fmt.Errorf("destination %q already exists; set overwrite=true to merge and replace entries", destination)
		}
	} else if !os.IsNotExist(err) {
		return CopyStats{}, fmt.Errorf("cannot stat destination: %w", err)
	}

	type dirCopyJob struct {
		source string
		target string
		mode   os.FileMode
		size   int64
	}
	type dirCreateJob struct {
		target string
		mode   os.FileMode
	}

	var dirs []dirCreateJob
	var files []dirCopyJob
	walkErr := filepath.Walk(source, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("copying directory trees containing symlinks is not supported: %s", path)
		}

		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := destination
		if rel != "." {
			target = filepath.Join(destination, rel)
		}

		if info.IsDir() {
			dirs = append(dirs, dirCreateJob{target: target, mode: info.Mode().Perm()})
			return nil
		}

		files = append(files, dirCopyJob{
			source: path,
			target: target,
			mode:   info.Mode().Perm(),
			size:   info.Size(),
		})
		return nil
	})
	if walkErr != nil {
		return CopyStats{}, walkErr
	}

	for _, dirJob := range dirs {
		if err := helper.MkdirAllClear(dirJob.target, dirJob.mode); err != nil {
			return CopyStats{}, fmt.Errorf("cannot create directory %q: %w", dirJob.target, err)
		}
	}

	for _, fileJob := range files {
		if dstInfo, err := os.Stat(fileJob.target); err == nil {
			if dstInfo.IsDir() {
				return CopyStats{}, fmt.Errorf("destination %q already exists as a directory", fileJob.target)
			}
			if !overwrite {
				return CopyStats{}, fmt.Errorf("destination %q already exists; set overwrite=true to replace it", fileJob.target)
			}
		} else if !os.IsNotExist(err) {
			return CopyStats{}, fmt.Errorf("cannot stat destination %q: %w", fileJob.target, err)
		}
	}

	type fileCopyResult struct {
		bytes int64
		err   error
	}
	results := make([]fileCopyResult, len(files))
	helper.RunIOBoundedParallel(len(files), func(i int) {
		n, err := helper.CopyFileDataN(files[i].source, files[i].target, files[i].mode)
		if err != nil {
			results[i].err = fmt.Errorf("%s -> %s: %w", files[i].source, files[i].target, err)
			return
		}
		results[i].bytes = n
	})

	stats := CopyStats{
		SourceKind: "directory",
		Files:      len(files),
		Dirs:       len(dirs),
	}
	for _, result := range results {
		if result.err != nil {
			return CopyStats{}, result.err
		}
		stats.Bytes += result.bytes
	}
	if stats.Dirs == 0 {
		if err := helper.MkdirAllClear(destination, srcInfo.Mode().Perm()); err != nil {
			return CopyStats{}, fmt.Errorf("cannot create destination directory: %w", err)
		}
		stats.Dirs = 1
	}
	return stats, nil
}
