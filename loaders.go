package bsa

import (
	"io"
	"io/fs"
	"mime"
	"path"
	"path/filepath"
	"strings"
)

// File resource struct
type ResourceFS struct {
	Source     fs.FS
	RootPath   string
	DontPrefix bool
}

func (r *ResourceFS) Absolute(filename string) string {
	if !r.DontPrefix {
		filename = path.Join(r.RootPath, filename)
	} else {
		filename = strings.TrimPrefix(filename, "/")
	}
	return filename
}

func (r *ResourceFS) ProbeType(filename string) (string, error) {
	filename = r.Absolute(filename)
	fd, err := r.Source.Open(filename)
	if err != nil {
		return "", err
	}
	defer fd.Close()
	ext := filepath.Ext(filename)
	t := mime.TypeByExtension(ext)
	return t, nil
}

func (r *ResourceFS) Size(filename string) (int64, error) {
	filename = r.Absolute(filename)
	s, err := fs.Stat(r.Source, filename)
	if err != nil {
		return 0, err
	}
	return s.Size(), nil
}

func (r *ResourceFS) WriteTo(filename string, w io.Writer) (int64, error) {
	filename = r.Absolute(filename)
	fd, err := r.Source.Open(filename)
	if err != nil {
		return 0, err
	}
	defer fd.Close()
	return io.Copy(w, fd)
}

func NewFSQueryLoader(_fs fs.FS, root string) QueryLoader {
	return NewResFSQueryLoader(&ResourceFS{
		Source:   _fs,
		RootPath: root,
	})
}

func NewResFSQueryLoader(r *ResourceFS) QueryLoader {
	return &fsQueryLoader{res: r}
}

type fsQueryLoader struct {
	res *ResourceFS
}

func (l *fsQueryLoader) Get(name string) (string, error) {
	fd, err := l.res.Source.Open(path.Join(l.res.RootPath, name))
	if err != nil {
		return "", err
	}
	defer fd.Close()
	b, err := io.ReadAll(fd)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
