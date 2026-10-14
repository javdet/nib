package executor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"

	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
)

// imageClient is the slice of the Docker client the image check needs.
type imageClient interface {
	ImageInspect(ctx context.Context, imageID string, opts ...client.ImageInspectOption) (image.InspectResponse, error)
	ImagePull(ctx context.Context, refStr string, options image.PullOptions) (io.ReadCloser, error)
}

// ensureImage pulls ref when the local daemon does not already have it.
//
// ContainerCreate does not pull the way `docker run` does — it answers "No such
// image" — so a host that has never run the configured agent image could not
// start an action at all. The pull is skipped when the image is present, which
// keeps a mutable tag pinned to whatever was pulled first, exactly like
// `docker run`. registryAuth yields the encoded X-Registry-Auth value, empty for
// an anonymous pull; it is called only when a pull happens, so a cached image
// never depends on the registry password secret still being readable.
func ensureImage(ctx context.Context, cli imageClient, ref string, registryAuth func() (string, error)) error {
	if _, err := cli.ImageInspect(ctx, ref); err == nil {
		return nil
	} else if !client.IsErrNotFound(err) {
		return fmt.Errorf("%w: inspect %s: %w", ErrImagePull, ref, err)
	}

	opts := image.PullOptions{}
	if registryAuth != nil {
		auth, err := registryAuth()
		if err != nil {
			return err
		}
		opts.RegistryAuth = auth
	}

	slog.Info("pulling executor image", "image", ref, "authenticated", opts.RegistryAuth != "")

	rc, err := cli.ImagePull(ctx, ref, opts)
	if err != nil {
		return fmt.Errorf("%w: %s: %w", ErrImagePull, ref, err)
	}
	defer rc.Close()

	if err := drainPullProgress(rc); err != nil {
		return fmt.Errorf("%w: %s: %w", ErrImagePull, ref, err)
	}

	slog.Info("pulled executor image", "image", ref)
	return nil
}

// pullImage makes cfg.Image available on the local daemon, logging in to its
// registry when the executor carries credentials for it.
func (s *Service) pullImage(ctx context.Context, cli imageClient, cfg Config) error {
	return ensureImage(ctx, cli, cfg.Image, func() (string, error) {
		creds, err := s.resolveRegistryCredentials(ctx, cfg)
		if err != nil {
			return "", err
		}
		return creds.dockerRegistryAuth()
	})
}

// drainPullProgress reads the pull to completion. Reading the stream is what
// waits for the layers, and the daemon reports a failed pull as an entry in the
// stream rather than as an HTTP error, so the body has to be decoded and not
// just discarded.
func drainPullProgress(r io.Reader) error {
	dec := json.NewDecoder(r)
	for {
		var msg struct {
			Error       string `json:"error"`
			ErrorDetail struct {
				Message string `json:"message"`
			} `json:"errorDetail"`
		}
		if err := dec.Decode(&msg); err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return fmt.Errorf("read pull progress: %w", err)
		}
		if detail := strings.TrimSpace(msg.ErrorDetail.Message); detail != "" {
			return errors.New(detail)
		}
		if e := strings.TrimSpace(msg.Error); e != "" {
			return errors.New(e)
		}
	}
}
