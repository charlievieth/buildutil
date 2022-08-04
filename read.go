// Copyright 2012 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package buildutil

import (
	"bufio"
	"bytes"
	"errors"
	"go/token"
	"io"
	"strings"
	"sync"
	"unicode/utf8"
)

type importReader struct {
	b    *bufio.Reader
	buf  []byte
	peek byte
	err  error
	eof  bool
	nerr int
	pos  token.Position
}

var bom = []byte{0xef, 0xbb, 0xbf}

var importReaderPool = sync.Pool{
	New: func() interface{} {
		return &importReader{
			b:   bufio.NewReader(nil),
			buf: make([]byte, 0, 512),
		}
	},
}

func putImportReader(r *importReader) {
	b := r.b
	buf := r.buf[:0]
	b.Reset(nil) // remove reference
	*r = importReader{b: b, buf: buf}
	importReaderPool.Put(r)
}

func newImportReader(name string, rd io.Reader) *importReader {
	r := importReaderPool.Get().(*importReader)
	r.b.Reset(rd)

	// Remove leading UTF-8 BOM.
	// Per https://golang.org/ref/spec#Source_code_representation:
	// a compiler may ignore a UTF-8-encoded byte order mark (U+FEFF)
	// if it is the first Unicode code point in the source text.
	if leadingBytes, err := r.b.Peek(3); err == nil && bytes.Equal(leadingBytes, bom) {
		r.b.Discard(3)
	}
	r.buf = r.buf[:0]
	r.pos = token.Position{
		Filename: name,
		Line:     1,
		Column:   1,
	}
	return r
}

func isIdent(c byte) bool {
	return 'A' <= c && c <= 'Z' || 'a' <= c && c <= 'z' || '0' <= c && c <= '9' || c == '_' || c >= utf8.RuneSelf
}

var (
	errSyntax = errors.New("syntax error")
	errNUL    = errors.New("unexpected NUL in input")
)

// syntaxError records a syntax error, but only if an I/O error has not already been recorded.
func (r *importReader) syntaxError() {
	if r.err == nil {
		r.err = errSyntax
	}
}

// readByte reads the next byte from the input, saves it in buf, and returns it.
// If an error occurs, readByte records the error in r.err and returns 0.
func (r *importReader) readByte() byte {
	c, err := r.b.ReadByte()
	if err == nil {
		r.buf = append(r.buf, c)
		if c == 0 {
			err = errNUL
		}
	}
	if err != nil {
		if err == io.EOF {
			r.eof = true
		} else if r.err == nil {
			r.err = err
		}
		c = 0
	}
	return c
}

// peekByte returns the next byte from the input reader but does not advance beyond it.
// If skipSpace is set, peekByte skips leading spaces and comments.
func (r *importReader) peekByte(skipSpace bool) byte {
	if r.err != nil {
		if r.nerr++; r.nerr > 10000 {
			panic("go/build: import reader looping")
		}
		return 0
	}

	// Use r.peek as first input byte.
	// Don't just return r.peek here: it might have been left by peekByte(false)
	// and this might be peekByte(true).
	c := r.peek
	if c == 0 {
		c = r.readByte()
	}
	for r.err == nil && !r.eof {
		if skipSpace {
			// For the purposes of this reader, semicolons are never necessary to
			// understand the input and are treated as spaces.
			switch c {
			case ' ', '\f', '\t', '\r', '\n', ';':
				c = r.readByte()
				continue

			case '/':
				c = r.readByte()
				if c == '/' {
					for c != '\n' && r.err == nil && !r.eof {
						c = r.readByte()
					}
				} else if c == '*' {
					var c1 byte
					for (c != '*' || c1 != '/') && r.err == nil {
						if r.eof {
							r.syntaxError()
						}
						c, c1 = c1, r.readByte()
					}
				} else {
					r.syntaxError()
				}
				c = r.readByte()
				continue
			}
		}
		break
	}
	r.peek = c
	return r.peek
}

// nextByte is like peekByte but advances beyond the returned byte.
func (r *importReader) nextByte(skipSpace bool) byte {
	c := r.peekByte(skipSpace)
	r.peek = 0
	return c
}

// readKeyword reads the given keyword from the input.
// If the keyword is not present, readKeyword records a syntax error.
func (r *importReader) readKeyword(kw string) {
	r.peekByte(true)
	for i := 0; i < len(kw); i++ {
		if r.nextByte(false) != kw[i] {
			r.syntaxError()
			return
		}
	}
	if isIdent(r.peekByte(false)) {
		r.syntaxError()
	}
}

// readIdent reads an identifier from the input.
// If an identifier is not present, readIdent records a syntax error.
func (r *importReader) readIdent() {
	c := r.peekByte(true)
	if !isIdent(c) {
		r.syntaxError()
		return
	}
	for isIdent(r.peekByte(false)) {
		r.peek = 0
	}
}

// readString reads a quoted string literal from the input.
// If an identifier is not present, readString records a syntax error.
func (r *importReader) readString() {
	switch r.nextByte(true) {
	case '`':
		for r.err == nil {
			if r.nextByte(false) == '`' {
				break
			}
			if r.eof {
				r.syntaxError()
			}
		}
	case '"':
		for r.err == nil {
			c := r.nextByte(false)
			if c == '"' {
				break
			}
			if r.eof || c == '\n' {
				r.syntaxError()
			}
			if c == '\\' {
				r.nextByte(false)
			}
		}
	default:
		r.syntaxError()
	}
}

// readImport reads an import clause - optional identifier followed by quoted string -
// from the input.
func (r *importReader) readImport() {
	c := r.peekByte(true)
	if c == '.' {
		r.peek = 0
	} else if isIdent(c) {
		r.readIdent()
	}
	r.readString()
}

// TODO: remove ??
//
// readComments is like io.ReadAll, except that it only reads the leading
// block of comments in the file.
func readComments(f io.Reader) ([]byte, error) {
	r := newImportReader("", f)
	defer putImportReader(r)
	r.peekByte(true)
	if r.err == nil && !r.eof {
		// Didn't reach EOF, so must have found a non-space byte. Remove it.
		r.buf = r.buf[:len(r.buf)-1]
	}
	return append([]byte(nil), r.buf...), r.err
}

// fileInfo records information learned about a file included in a build.
type fileInfo struct {
	name   string // full name including dir
	header []byte
}

// readPackageClause is like readImports, except that it stops reading after the
// package clause.
func readPackageClause(f io.Reader) ([]byte, error) {
	r := newImportReader("dummy.go", f)
	defer putImportReader(r)
	r.readKeyword("package")
	r.readIdent()
	r.readByte()

	// If we stopped successfully before EOF, we read a byte that told us we were done.
	// Return all but that last byte, which would cause a syntax error if we let it through.
	if r.err == nil && !r.eof {
		buf := append([]byte(nil), r.buf[:len(r.buf)-1]...)
		return buf, nil
	}
	buf := append([]byte(nil), r.buf...)
	err := r.err
	return buf, err
}

// readGoInfo expects a Go file as input and reads the file up to and including the import section.
// It records what it learned in *info.
// If info.fset is non-nil, readGoInfo parses the file and sets info.parsed, info.parseErr,
// info.imports, info.embeds, and info.embedErr.
//
// It only returns an error if there are problems reading the file,
// not for syntax errors in the file itself.
func readGoInfo(f io.Reader, info *fileInfo) error {
	r := newImportReader(info.name, f)
	defer putImportReader(r)

	r.readKeyword("package")
	r.readIdent()
	for r.peekByte(true) == 'i' {
		r.readKeyword("import")
		if r.peekByte(true) == '(' {
			r.nextByte(false)
			for r.peekByte(true) != ')' && r.err == nil {
				r.readImport()
			}
			r.nextByte(false)
		} else {
			r.readImport()
		}
	}

	info.header = r.buf

	// If we stopped successfully before EOF, we read a byte that told us we were done.
	// Return all but that last byte, which would cause a syntax error if we let it through.
	if r.err == nil && !r.eof {
		info.header = r.buf[:len(r.buf)-1]
	}

	// If we stopped for a syntax error, consume the whole file so that
	// we are sure we don't change the errors that go/parser returns.
	if r.err == errSyntax {
		r.err = nil
		for r.err == nil && !r.eof {
			r.readByte()
		}
		info.header = r.buf
	}
	// Make a copy of the header since we re-use readers
	if n := len(info.header); n != 0 {
		b := make([]byte, n)
		copy(b, info.header)
		info.header = b
	} else {
		info.header = nil
	}
	if r.err != nil {
		return r.err
	}
	return nil
}

// cut is the same as strings.Cut
func cut(s, sep string) (before, after string, found bool) {
	if i := strings.Index(s, sep); i >= 0 {
		return s[:i], s[i+len(sep):], true
	}
	return s, "", false
}

// readImports is like ioutil.ReadAll, except that it expects a Go file as input
// and stops reading the input once the imports have completed.
func readImports(f io.Reader, reportSyntaxError bool, imports *[]string) ([]byte, error) {
	info := fileInfo{name: "dummy.go"}
	if err := readGoInfo(f, &info); err != nil {
		return nil, err
	}
	return info.header, nil
}

var (
	packageBytes   = []byte("package")
	starSlashBytes = []byte("*/")
)

func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r'
}

func readPackageName(b []byte) (string, error) {
	const minLen = len("package _")

	// trim left whitespace
	for len(b) > 0 && isSpace(b[0]) {
		b = b[1:]
	}

Loop:
	for len(b) >= minLen {
		c := b[0]
		switch c {
		case ' ', '\t', '\n', '\r', '\f', ';':
			b = b[1:]
		case '/':
			b = b[1:]
			c = b[0]
			switch c {
			case '/':
				n := bytes.IndexByte(b, '\n')
				if n == -1 || n == len(b)-1 {
					return "", errSyntax
				}
				b = b[n+1:]
			case '*':
				n := bytes.Index(b, starSlashBytes)
				if n == -1 || n == len(b)-2 {
					return "", errSyntax
				}
				b = b[n+2:]
			default:
				return "", errSyntax
			}
		default:
			break Loop
		}
	}

	if len(b) >= minLen && bytes.HasPrefix(b, packageBytes) {
		b = b[len("package"):]
		if !isSpace(b[0]) {
			return "", errSyntax
		}
		for len(b) > 0 && isSpace(b[0]) {
			b = b[1:]
		}
		i := 0
		for ; i < len(b) && isIdent(b[i]); i++ {
		}
		if i == 0 {
			// panic(fmt.Sprintf("len(b): %d i: %d\n", len(b), i))
			return "", errSyntax
		}
		return string(b[:i]), nil
	}

	return "", errSyntax
}
