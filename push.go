package led

import (
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"

	"github.com/SherClockHolmes/webpush-go"
	"github.com/joho/godotenv"
)

func (f *FileHandler) webPush(w http.ResponseWriter, req *http.Request) {
	d := map[string]string{
		"title": req.FormValue("title"),
		"body":  req.FormValue("body"),
		"data":  req.FormValue("data"),
	}
	m, err := json.Marshal(d)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(err.Error()))
		return
	}

	var s webpush.Subscription
	subs := req.FormValue("subs")
	if err := json.Unmarshal([]byte(subs), &s); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(err.Error()))
		return
	}

	envs, err := godotenv.Read(filepath.Join(f.Root, "env"))
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}
	o := webpush.Options{
		TTL:             86400,
		Subscriber:      envs["PUSH_SUBSCRIBER"],
		VAPIDPublicKey:  envs["PUSH_VAPID_PUB"],
		VAPIDPrivateKey: envs["PUSH_VAPID_PRI"],
	}

	r, err := webpush.SendNotification(m, &s, &o)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}
	defer r.Body.Close()

	w.WriteHeader(r.StatusCode)
	io.Copy(w, r.Body)
}
