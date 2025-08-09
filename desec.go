package led

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

func deSecZone(name, token string) (zone string, err error) {
	zone = "; desec-token: " + token + "\n" +
		"@ NS ns1.desec.io.\n" +
		"@ NS ns2.desec.org.\n"

	api := "https://desec.io/api/v1/domains/" + name + "/"
	req, err := http.NewRequest("GET", api, nil)
	if err != nil {
		return
	}
	req.Header.Set("Authorization", "Token "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		err = errors.New("invalid desec token")
		return
	}
	var x struct {
		Name string `json:"name"`
		Keys []struct {
			DS []string `json:"ds"`
		} `json:"keys"`
	}
	err = json.NewDecoder(resp.Body).Decode(&x)
	if err != nil {
		err = errors.New("invalid response from desec")
		return
	}

	for _, key := range x.Keys {
		for _, ds := range key.DS {
			s := strings.Fields(ds)
			if s[2] == "2" {
				zone += "@ DS " + ds + "\n"
			}
		}
	}
	return
}
