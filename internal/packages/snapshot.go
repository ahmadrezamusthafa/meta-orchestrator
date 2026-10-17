package packages

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Snapshots store the files a package owns as paths relative to the operator's home folder, and
// restoring one writes only inside that folder: entries with absolute paths, ".." or that would
// pass through or create a symbolic link pointing outside it are refused.

// resolveFootprint expands the patterns and returns the existing paths they match, relative to
// root, sorted and without paths nested in another match.
func resolveFootprint(patterns []string, root, dir string) ([]string, error) {
	seen := map[string]bool{}
	var rels []string
	for _, p := range patterns {
		matches, err := filepath.Glob(expand(p, root, dir))
		if err != nil {
			return nil, fmt.Errorf("footprint %q: %w", p, err)
		}
		for _, m := range matches {
			rel, err := relInside(root, m)
			if err != nil {
				return nil, err
			}
			if !seen[rel] {
				seen[rel] = true
				rels = append(rels, rel)
			}
		}
	}
	sort.Strings(rels)
	var out []string
	for _, r := range rels {
		if len(out) > 0 && strings.HasPrefix(r, out[len(out)-1]+string(filepath.Separator)) {
			continue
		}
		out = append(out, r)
	}
	return out, nil
}

// relInside returns p relative to root, refusing root itself and anything outside it.
func relInside(root, p string) (string, error) {
	rel, err := filepath.Rel(root, p)
	if err != nil {
		return "", err
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("%s is not inside %s", p, root)
	}
	return rel, nil
}

// writeSnapshot archives the given paths (relative to root) into dest. Symbolic links are stored
// as links, never followed.
func writeSnapshot(dest, root string, rels []string) (err error) {
	if err := os.MkdirAll(filepath.Dir(dest), stateFolderMode); err != nil {
		return err
	}
	tmp := dest + ".partial"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, stateFileMode)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = os.Remove(tmp)
		}
	}()
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	for _, rel := range rels {
		werr := filepath.WalkDir(filepath.Join(root, rel), func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			info, err := os.Lstat(path)
			if err != nil {
				return err
			}
			name, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			link := ""
			if info.Mode()&os.ModeSymlink != 0 {
				if link, err = os.Readlink(path); err != nil {
					return err
				}
			} else if !info.Mode().IsRegular() && !info.IsDir() {
				return nil // sockets, devices: never part of a package
			}
			hdr, err := tar.FileInfoHeader(info, link)
			if err != nil {
				return err
			}
			hdr.Name = filepath.ToSlash(name)
			if info.IsDir() {
				hdr.Name += "/"
			}
			hdr.Uname, hdr.Gname = "", ""
			if err := tw.WriteHeader(hdr); err != nil {
				return err
			}
			if info.Mode().IsRegular() {
				src, err := os.Open(path)
				if err != nil {
					return err
				}
				_, err = io.Copy(tw, src)
				src.Close()
				return err
			}
			return nil
		})
		if werr != nil {
			tw.Close()
			gz.Close()
			f.Close()
			return werr
		}
	}
	if err := tw.Close(); err != nil {
		f.Close()
		return err
	}
	if err := gz.Close(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, dest)
}

// ErrUnsafeArchive means a snapshot entry would write outside the home folder.
var ErrUnsafeArchive = errors.New("unsafe snapshot entry")

// snapshotPaths lists the top-level paths a snapshot holds (relative to root, OS separators).
func snapshotPaths(src string) ([]string, error) {
	var names []string
	err := readSnapshot(src, func(hdr *tar.Header, _ io.Reader) error {
		names = append(names, filepath.FromSlash(strings.TrimSuffix(hdr.Name, "/")))
		return nil
	})
	return names, err
}

func readSnapshot(src string, fn func(*tar.Header, io.Reader) error) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if err := fn(hdr, tr); err != nil {
			return err
		}
	}
}

// safeEntryPath validates an archive name and returns its absolute destination under root.
func safeEntryPath(root, name string) (string, error) {
	clean := strings.TrimSuffix(name, "/")
	if clean == "" || strings.HasPrefix(clean, "/") || filepath.IsAbs(filepath.FromSlash(clean)) || strings.Contains(clean, "\\") {
		return "", fmt.Errorf("%w: %q", ErrUnsafeArchive, name)
	}
	for _, part := range strings.Split(clean, "/") {
		if part == "" || part == "." || part == ".." {
			return "", fmt.Errorf("%w: %q", ErrUnsafeArchive, name)
		}
	}
	return filepath.Join(root, filepath.FromSlash(clean)), nil
}

// noSymlinkParents refuses a destination whose parent folders (below root) include a symbolic
// link, which could redirect the write outside root.
func noSymlinkParents(root, dest string) error {
	rel, err := filepath.Rel(root, filepath.Dir(dest))
	if err != nil {
		return err
	}
	if rel == "." {
		return nil
	}
	cur := root
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		cur = filepath.Join(cur, part)
		info, err := os.Lstat(cur)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%w: %s passes through the symbolic link %s", ErrUnsafeArchive, dest, cur)
		}
	}
	return nil
}

// validateSnapshot checks every entry before anything is removed, so an unsafe archive leaves the
// current files untouched.
func validateSnapshot(src, root string) error {
	return readSnapshot(src, func(hdr *tar.Header, _ io.Reader) error {
		dest, err := safeEntryPath(root, hdr.Name)
		if err != nil {
			return err
		}
		if err := noSymlinkParents(root, dest); err != nil {
			return err
		}
		switch hdr.Typeflag {
		case tar.TypeDir, tar.TypeReg:
		case tar.TypeSymlink:
			if err := safeLinkTarget(root, dest, hdr.Linkname); err != nil {
				return err
			}
		default:
			return fmt.Errorf("%w: %q has unsupported type %c", ErrUnsafeArchive, hdr.Name, hdr.Typeflag)
		}
		return nil
	})
}

// safeLinkTarget allows relative link targets that stay inside root.
func safeLinkTarget(root, dest, target string) error {
	if target == "" || filepath.IsAbs(target) {
		return fmt.Errorf("%w: link %s → %q leaves the home folder", ErrUnsafeArchive, dest, target)
	}
	resolved := filepath.Join(filepath.Dir(dest), target)
	if _, err := relInside(root, resolved); err != nil {
		return fmt.Errorf("%w: link %s → %q leaves the home folder", ErrUnsafeArchive, dest, target)
	}
	return nil
}

// restoreSnapshot replaces the current files with the snapshot: it validates the archive, removes
// the current footprint paths and the snapshot's own top-level paths, then extracts it.
func restoreSnapshot(src, root string, current []string) error {
	if err := validateSnapshot(src, root); err != nil {
		return err
	}
	tops, err := snapshotPaths(src)
	if err != nil {
		return err
	}
	remove := append([]string(nil), current...)
	remove = append(remove, topLevel(tops)...)
	for _, rel := range remove {
		p, err := safeEntryPath(root, filepath.ToSlash(rel))
		if err != nil {
			return err
		}
		if err := noSymlinkParents(root, p); err != nil {
			return err
		}
		if err := os.RemoveAll(p); err != nil {
			return err
		}
	}
	return readSnapshot(src, func(hdr *tar.Header, r io.Reader) error {
		dest, err := safeEntryPath(root, hdr.Name)
		if err != nil {
			return err
		}
		if err := noSymlinkParents(root, dest); err != nil {
			return err
		}
		mode := os.FileMode(hdr.Mode).Perm()
		switch hdr.Typeflag {
		case tar.TypeDir:
			if mode == 0 {
				mode = 0o755
			}
			return os.MkdirAll(dest, mode|0o700)
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
				return err
			}
			f, err := os.OpenFile(dest, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode|0o600)
			if err != nil {
				return err
			}
			if _, err := io.Copy(f, r); err != nil {
				f.Close()
				return err
			}
			return f.Close()
		case tar.TypeSymlink:
			if err := safeLinkTarget(root, dest, hdr.Linkname); err != nil {
				return err
			}
			if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
				return err
			}
			_ = os.Remove(dest)
			return os.Symlink(hdr.Linkname, dest)
		}
		return fmt.Errorf("%w: %q", ErrUnsafeArchive, hdr.Name)
	})
}

// topLevel keeps the outermost paths of an archive listing (entries are written parent first).
func topLevel(paths []string) []string {
	var out []string
	for _, p := range paths {
		nested := false
		for _, o := range out {
			if strings.HasPrefix(p, o+string(filepath.Separator)) {
				nested = true
				break
			}
		}
		if !nested {
			out = append(out, p)
		}
	}
	return out
}

// removeFootprint deletes the given paths (relative to root).
func removeFootprint(root string, rels []string) error {
	for _, rel := range rels {
		p, err := safeEntryPath(root, filepath.ToSlash(rel))
		if err != nil {
			return err
		}
		if err := noSymlinkParents(root, p); err != nil {
			return err
		}
		if err := os.RemoveAll(p); err != nil {
			return err
		}
	}
	return nil
}
