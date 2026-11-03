package main

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/librecosta/gofurry/services/api/internal/platform"
	"github.com/librecosta/gofurry/services/api/internal/storage"
	"github.com/spf13/cobra"
)

// An operator-only, create-once import command, not an HTTP upload endpoint.
func backgroundImportCommand(config *platform.Config) *cobra.Command {
	return &cobra.Command{
		Use: "background-import FILE", Short: "Import and verify one development AVIF/WebP background", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if config.Server.Environment != "development" || config.Storage == nil {
				return errors.New("background-import requires development YAML with explicit storage configuration")
			}
			file, err := os.Open(args[0])
			if err != nil {
				return errors.New("cannot open background file")
			}
			defer file.Close()
			const maxBytes = 20 * 1024 * 1024
			data, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
			if err != nil || len(data) < 12 || len(data) > maxBytes {
				return errors.New("background must be 12 bytes..20 MiB")
			}
			ext := strings.ToLower(filepath.Ext(args[0]))
			contentType := "image/" + strings.TrimPrefix(ext, ".")
			valid := ext == ".webp" && string(data[:4]) == "RIFF" && string(data[8:12]) == "WEBP"
			valid = valid || ext == ".avif" && string(data[4:8]) == "ftyp" && bytes.Contains(data[8:min(64, len(data))], []byte("avif"))
			if !valid {
				return errors.New("file must contain AVIF or WebP matching its extension")
			}
			hash := fmt.Sprintf("%x", sha256.Sum256(data))
			key := "start/backgrounds/" + hash + ext
			store := storage.New(*config.Storage)
			err = store.PutNew(cmd.Context(), key, contentType, hash, bytes.NewReader(data))
			if err != nil && !errors.Is(err, storage.ErrExists) {
				return err
			}
			// Existing objects are never overwritten, but must pass the same checks.
			head, err := store.Head(cmd.Context(), key)
			if err != nil {
				return err
			}
			body, _, err := store.Get(cmd.Context(), key)
			if err != nil {
				return err
			}
			stored, readErr := io.ReadAll(io.LimitReader(body, maxBytes+1))
			body.Close()
			if readErr != nil || sha256.Sum256(stored) != sha256.Sum256(data) || head.Bytes != int64(len(data)) || head.ContentType != contentType || head.SHA256 != hash {
				return errors.New("S3 HEAD/GET verification failed")
			}
			req, err := http.NewRequestWithContext(cmd.Context(), http.MethodGet, head.URL, nil)
			if err != nil {
				return errors.New("invalid public asset URL")
			}
			response, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
			if err != nil {
				return errors.New("public CDN GET failed")
			}
			defer response.Body.Close()
			public, err := io.ReadAll(io.LimitReader(response.Body, maxBytes+1))
			if err != nil || response.StatusCode != http.StatusOK || response.Header.Get("Content-Type") != contentType || sha256.Sum256(public) != sha256.Sum256(data) {
				return errors.New("public CDN content/type/hash verification failed")
			}
			output, err := platform.MarshalJSON(head)
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), string(output))
			return nil
		},
	}
}
