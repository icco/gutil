// Package render writes JSON responses, logging rather than returning a write
// failure so handlers can call it as a final statement.
package render

import (
	"io"

	"github.com/unrolled/render"
	"go.uber.org/zap"
)

var (
	r = render.New(render.Options{
		IndentJSON:    true,
		StreamingJSON: true,
	})
)

// JSON logs a json object to the response.
func JSON(log *zap.SugaredLogger, w io.Writer, status int, v interface{}) {
	if err := r.JSON(w, status, v); err != nil {
		log.Errorw("could not write response", zap.Error(err))
	}
}
