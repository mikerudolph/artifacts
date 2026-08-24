package api

import (
	"encoding/json"
	"net/http"

	"github.com/mikerudolph/artifacts/internal/api/envelope"
)

func writeOK(w http.ResponseWriter, status int, v any) {
	envelope.Write(w, status, envelope.OK(v))
}

func writeOKInfo(w http.ResponseWriter, v any, info any) {
	envelope.Write(w, http.StatusOK, envelope.OKInfo(v, info))
}

func writeErr(w http.ResponseWriter, err error) {
	status, apiErr := envelope.FromDomain(err)
	_, body := envelope.Fail(status, apiErr)
	envelope.Write(w, status, body)
}

func decodeJSON(r *http.Request, v any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}
