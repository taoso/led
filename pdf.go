package led

import (
	"io"
	"net/http"
	"os"
	"os/exec"
)

func (f *FileHandler) pdf2txt(w http.ResponseWriter, req *http.Request) {
	err := req.ParseMultipartForm(10 << 20)
	if err != nil {
		http.Error(w, "Unable to parse form", http.StatusBadRequest)
		return
	}

	pdf, _, err := req.FormFile("pdf")
	if err != nil {
		http.Error(w, "Unable to get the file", http.StatusBadRequest)
		return
	}
	defer pdf.Close()

	tmp, err := os.CreateTemp("", "uploaded*.pdf")
	if err != nil {
		http.Error(w, "Unable to create a temporary file", http.StatusInternalServerError)
		return
	}
	defer tmp.Close()
	defer os.Remove(tmp.Name())

	if _, err = io.Copy(tmp, pdf); err != nil {
		http.Error(w, "Unable to save the file", http.StatusInternalServerError)
		return
	}

	txt := tmp.Name() + ".txt"

	cmd := exec.Command("/usr/local/bin/pdf2txt", tmp.Name(), txt)
	output, err := cmd.CombinedOutput()
	if err != nil {
		http.Error(w, "pdf2txt err: "+err.Error()+"\n"+string(output), http.StatusInternalServerError)
		return
	}
	defer os.Remove(txt)

	http.ServeFile(w, req, txt)
}
