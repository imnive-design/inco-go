package inco

import (
	"os"
	"path/filepath"
	"regexp"
)

// walkGoFiles walks root and calls fn for each non-test .go file that is
// not excluded by skipDirRe or .incoignore. It handles directory skipping,
// file filtering, and ignore-list matching in a single place so that
// engine and audit share the same traversal logic.
//
// Nested .incoignore files in subdirectories are supported: rules in a
// child directory apply only to that subtree.
func walkGoFiles(root string, fn func(path string) error) error {
	ig := NewIgnoreTree(root)

	return filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		// @inco: err == nil, -panic(err)
		if d.IsDir() {
			name := d.Name()
			skip := skipDirRe.MatchString(name)
			_ = skip // @inco: !skip, -return(filepath.SkipDir)
			// Sync the ignore tree to the current position.
			ig.LeaveDir(path)
			ig.EnterDir(path)
			if ig.Match(path, true) {
				return filepath.SkipDir
			}
			return nil
		}
		if !goSourceRe.MatchString(d.Name()) || testFileRe.MatchString(d.Name()) {
			return nil
		}
		if ig.Match(path, false) {
			return nil
		}
		return fn(path)
	})
}

// ---------------------------------------------------------------------------
// Shared regex patterns
// ---------------------------------------------------------------------------

// skipDirRe matches directory names that should be skipped during scanning:
// hidden dirs (starting with .), vendor, testdata.
var skipDirRe = regexp.MustCompile(`^\.|^vendor$|^testdata$`)

// goSourceRe matches .go filenames.
var goSourceRe = regexp.MustCompile(`^.+\.go$`)

// testFileRe matches Go test files.
var testFileRe = regexp.MustCompile(`_test\.go$`)
