package api

import (
	"encoding/json"
	"errors"
	"io"
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

const maxJSONBody = 2 << 20

func decodeJSON(w http.ResponseWriter, r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxJSONBody)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return err
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("request body must contain one JSON value")
		}
		return err
	}
	return nil
}
