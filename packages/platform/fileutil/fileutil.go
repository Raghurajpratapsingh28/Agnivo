// Package fileutil provides safe filesystem helpers: atomic file writes,
// existence checks, and path containment validation to prevent traversal
// attacks when building paths from untrusted input.
package fileutil

import (
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/agnivo/agnivo/packages/platform/errors"
)

// Exists reports whether path exists (file or directory).
func Exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// IsDir reports whether path exists and is a directory.
func IsDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// WriteAtomic writes data to path atomically by writing to a temporary file in
// the same directory and renaming it into place. Readers therefore never
// observe a partially written file. The file is created with the given perm.
func WriteAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return errors.Wrap(err, errors.CodeInternal, "fileutil: create temp")
	}
	tmpName := tmp.Name()
	// Best-effort cleanup if anything below fails before the rename.
	defer func() { _ = os.Remove(tmpName) }()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return errors.Wrap(err, errors.CodeInternal, "fileutil: write temp")
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return errors.Wrap(err, errors.CodeInternal, "fileutil: sync temp")
	}
	if err := tmp.Close(); err != nil {
		return errors.Wrap(err, errors.CodeInternal, "fileutil: close temp")
	}
	if err := os.Chmod(tmpName, perm); err != nil {
		return errors.Wrap(err, errors.CodeInternal, "fileutil: chmod temp")
	}
	if err := os.Rename(tmpName, path); err != nil {
		return errors.Wrap(err, errors.CodeInternal, "fileutil: rename")
	}
	return nil
}

// CopyFile copies the contents of src to dst, creating dst with perm. It does
// not preserve ownership or timestamps.
func CopyFile(src, dst string, perm os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return errors.Wrap(err, errors.CodeInternal, "fileutil: open source")
	}
	defer func() { _ = in.Close() }()

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return errors.Wrap(err, errors.CodeInternal, "fileutil: open dest")
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return errors.Wrap(err, errors.CodeInternal, "fileutil: copy")
	}
	if err := out.Close(); err != nil {
		return errors.Wrap(err, errors.CodeInternal, "fileutil: close dest")
	}
	return nil
}

// SafeJoin joins base and untrusted path elements, guaranteeing the result
// stays within base. It rejects inputs that would escape via "..". Use it when
// constructing filesystem paths from user-supplied names.
func SafeJoin(base string, elem ...string) (string, error) {
	cleanBase := filepath.Clean(base)
	joined := filepath.Join(append([]string{cleanBase}, elem...)...)
	if joined != cleanBase && !strings.HasPrefix(joined, cleanBase+string(os.PathSeparator)) {
		return "", errors.Newf(errors.CodeInvalidArgument, "fileutil: path %q escapes base %q", joined, cleanBase)
	}
	return joined, nil
}
