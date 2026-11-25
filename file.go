package led

import (
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

type FileHandler struct {
	Root string
	Name string
	fs   http.Handler
}

func NewHandler(root string, name string) *FileHandler {
	path := filepath.Join(root, name)

	if _, err := os.Stat(path); os.IsNotExist(err) {
		path = filepath.Join(root, "default")
	}

	return &FileHandler{
		Root: path,
		Name: name,
		fs:   http.FileServer(leDir{http.Dir(path)}),
	}
}

func (h *FileHandler) Rewritten(w http.ResponseWriter, req *http.Request) bool {
	// Google AdSense
	if strings.HasPrefix(req.URL.Path, "/ads.txt") {
		return false
	}

	b, err := os.ReadFile(path.Join(h.Root, "rewrite.txt"))

	if os.IsNotExist(err) {
		return false
	}

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return false
	}

	lines := strings.TrimSpace(string(b))
	for line := range strings.SplitSeq(lines, "\n") {
		parts := strings.Split(line, " -> ")

		if len(parts) < 2 {
			continue
		}

		oldURL := parts[0]
		newURL := parts[1]

		if strings.HasPrefix(req.URL.Path, oldURL) {
			code := http.StatusMovedPermanently

			if strings.HasPrefix(newURL, "@") {
				newURL = newURL[1:]
				code = http.StatusTemporaryRedirect
			} else if strings.HasPrefix(newURL, "~") {
				newURL = newURL[1:] + req.URL.RequestURI()
			}

			http.Redirect(w, req, newURL, code)
			return true
		}
	}

	return false
}

type leDir struct {
	fs http.FileSystem
}

func (d leDir) Open(path string) (f http.File, err error) {
	if strings.HasSuffix(path, "/env") || strings.HasSuffix(path, ".md") {
		return nil, fs.ErrPermission
	}

	if strings.HasSuffix(path, ".html") {
		htm := strings.TrimSuffix(path, "l")
		if f, err = d.fs.Open(htm); err == nil {
			return f, nil
		}
	}

	f, err = d.fs.Open(path)
	if err != nil {
		return nil, err
	}
	s, err := f.Stat()
	if s.IsDir() {
		if a, err := d.fs.Open(path + "/.autoindex"); err == nil {
			a.Close()
			return f, nil
		}
		if _, err := d.fs.Open(path + "/index.htm"); err != nil {
			if _, err := d.fs.Open(path + "/index.html"); err != nil {
				return nil, err
			}
		}
	}

	return f, nil
}
