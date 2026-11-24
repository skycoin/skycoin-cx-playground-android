package benchsuite

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// UploadResults POSTs doc in the text form to url and returns the response
// body.
func UploadResults(ctx context.Context, url string, doc *ResultsDocument) (string, error) {
	body, err := EncodeResultsDocument(doc)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "text/plain; charset=utf-8")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	reply, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode/100 != 2 {
		return "", fmt.Errorf("%s: %s", resp.Status, strings.TrimSpace(string(reply)))
	}
	return strings.TrimSpace(string(reply)), nil
}
