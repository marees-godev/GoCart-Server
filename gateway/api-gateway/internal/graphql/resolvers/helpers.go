package resolvers

import (
	"encoding/base64"
	"fmt"
	"io"

	"github.com/99designs/gqlgen/graphql"
)

// processUpload converts a multipart graphql.Upload stream into a data URI for service-layer processing.
func processUpload(upload *graphql.Upload) (string, error) {
	if upload == nil || upload.File == nil {
		return "", nil
	}
	data, err := io.ReadAll(upload.File)
	if err != nil {
		return "", err
	}
	contentType := upload.ContentType
	if contentType == "" {
		contentType = "image/png"
	}
	encoded := base64.StdEncoding.EncodeToString(data)
	return fmt.Sprintf("data:%s;base64,%s", contentType, encoded), nil
}
