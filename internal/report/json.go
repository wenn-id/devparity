package report

import (
	"encoding/json"
	"io"

	"github.com/wenn-id/devparity/internal/model"
)

func JSON(w io.Writer, value model.Report) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}
