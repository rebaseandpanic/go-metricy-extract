// Package sourceloc provides directory traversal and repository-root
// resolution helpers for the extractor pipeline.
package sourceloc

import (
	"bufio"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// generatedRe matches the canonical Go-toolchain "DO NOT EDIT" marker used
// to flag machine-generated source files. Compiled once so IsGenerated does
// not re-parse the regex on every file visit.
//
// Reference: https://pkg.go.dev/cmd/go#hdr-Generate_Go_files_by_processing_source
var generatedRe = regexp.MustCompile(`^// Code generated .* DO NOT EDIT\.$`)

// maxGeneratedScanBytes is the upper bound on bytes read from a file while
// probing for the generated-code marker. Caps I/O for pathological inputs
// (giant single-line files) while still giving the marker room to appear
// after a license header.
const maxGeneratedScanBytes = 64 * 1024

// maxGeneratedScanLines caps the number of lines inspected for the marker.
// The Go convention places the marker in the first comment block of the
// file; 20 lines easily covers a 10-line license header plus leading blank.
const maxGeneratedScanLines = 20

// skipDirNames is the set of directory basenames that are always excluded
// from the walk. Covers vendored deps, common VCS/IDE metadata, and Go's
// testdata convention.
var skipDirNames = map[string]struct{}{
	"vendor":   {},
	".git":     {},
	".idea":    {},
	".vscode":  {},
	"testdata": {},
}

// WalkGoFiles walks root recursively and returns all .go files it finds,
// excluding test files, vendored packages, generated code, and hidden paths.
//
// Excluded:
//   - *_test.go                — Go test files
//   - paths containing vendor/ — vendored dependencies
//   - paths inside .git/, .idea/, .vscode/ — VCS/IDE dirs
//   - filenames starting with _ — Go build-excluded files (e.g. _generated.go)
//   - paths inside directories named testdata/ — Go convention for test fixtures
//   - directories whose name starts with "." or "_" (other than the root itself)
//   - files whose first comment line matches the canonical "Code generated
//     ... DO NOT EDIT." marker within the first 20 lines / 64 KiB.
//   - symlinked directories are not followed (matches filepath.WalkDir default)
//
// Returns paths in deterministic (lexicographic) order.
func WalkGoFiles(root string) ([]string, error) {
	var out []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			// Unreadable non-root directory: skip it, keep walking. Root itself or
			// file-level errors remain fatal.
			if d != nil && d.IsDir() && path != root {
				return filepath.SkipDir
			}
			return walkErr
		}
		// The walker visits the root itself first. Never filter it out —
		// otherwise passing a legitimate start dir named e.g. "_archive" or
		// "testdata" would return zero files when the user explicitly asked
		// for that path.
		if path == root {
			return nil
		}
		name := d.Name()
		if d.IsDir() {
			if _, skip := skipDirNames[name]; skip {
				return filepath.SkipDir
			}
			if strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") {
				return filepath.SkipDir
			}
			return nil
		}
		// Regular file: accept .go, reject tests, reject underscore-prefixed
		// (Go build excludes them anyway).
		if !strings.HasSuffix(name, ".go") {
			return nil
		}
		if strings.HasSuffix(name, "_test.go") {
			return nil
		}
		if strings.HasPrefix(name, "_") {
			return nil
		}
		gen, err := IsGenerated(path)
		if err != nil {
			return err
		}
		if gen {
			return nil
		}
		out = append(out, path)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(out)
	return out, nil
}

// IsGenerated reports whether the file at path is auto-generated per the
// Go toolchain convention. Reads up to the first 64 KiB or 20 lines
// (whichever comes first) and checks each line against the canonical
// "// Code generated ... DO NOT EDIT." marker.
func IsGenerated(path string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()

	lr := &io.LimitedReader{R: f, N: maxGeneratedScanBytes}
	scanner := bufio.NewScanner(lr)
	// Allow a single long line up to the byte cap; default Scanner buffer
	// caps at 64 KiB which matches maxGeneratedScanBytes — explicit for clarity.
	scanner.Buffer(make([]byte, 0, 4096), maxGeneratedScanBytes)
	for i := 0; i < maxGeneratedScanLines && scanner.Scan(); i++ {
		if generatedRe.MatchString(scanner.Text()) {
			return true, nil
		}
	}
	if err := scanner.Err(); err != nil && !errors.Is(err, bufio.ErrTooLong) {
		return false, err
	}
	return false, nil
}
