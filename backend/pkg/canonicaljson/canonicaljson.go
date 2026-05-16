package canonicaljson

import (
	"encoding/json"

	"github.com/gowebpki/jcs"
)

func Bytes(data any) ([]byte, error) {
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}

	return jcs.Transform(jsonBytes)
}
