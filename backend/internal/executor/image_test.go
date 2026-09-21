package executor

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
	"github.com/docker/docker/errdefs"
)

// fakeImageClient records what ensureImage asked the daemon for and replays a
// canned inspect result and pull stream.
type fakeImageClient struct {
	inspectErr error
	pullBody   string
	pullErr    error

	inspected []string
	pulled    []string
	closed    bool
}

func (f *fakeImageClient) ImageInspect(_ context.Context, ref string, _ ...client.ImageInspectOption) (image.InspectResponse, error) {
	f.inspected = append(f.inspected, ref)
	return image.InspectResponse{}, f.inspectErr
}

func (f *fakeImageClient) ImagePull(_ context.Context, ref string, _ image.PullOptions) (io.ReadCloser, error) {
	f.pulled = append(f.pulled, ref)
	if f.pullErr != nil {
		return nil, f.pullErr
	}
	return &trackingReadCloser{Reader: strings.NewReader(f.pullBody), closed: &f.closed}, nil
}

type trackingReadCloser struct {
	io.Reader
	closed *bool
}

func (t *trackingReadCloser) Close() error {
	*t.closed = true
	return nil
}

func TestEnsureImagePresentSkipsPull(t *testing.T) {
	t.Parallel()

	cli := &fakeImageClient{}
	if err := ensureImage(context.Background(), cli, "javdet/nib-agent:v0.8.0"); err != nil {
		t.Fatalf("ensureImage() error = %v, want nil", err)
	}
	if len(cli.pulled) != 0 {
		t.Fatalf("pulled = %v, want no pull", cli.pulled)
	}
	if len(cli.inspected) != 1 || cli.inspected[0] != "javdet/nib-agent:v0.8.0" {
		t.Fatalf("inspected = %v, want [javdet/nib-agent:v0.8.0]", cli.inspected)
	}
}

func TestEnsureImageMissingPulls(t *testing.T) {
	t.Parallel()

	cli := &fakeImageClient{
		inspectErr: errdefs.NotFound(errors.New("No such image: javdet/nib-agent:v0.8.0")),
		pullBody:   `{"status":"Pulling from javdet/nib-agent"}` + "\n" + `{"status":"Status: Downloaded newer image"}`,
	}
	if err := ensureImage(context.Background(), cli, "javdet/nib-agent:v0.8.0"); err != nil {
		t.Fatalf("ensureImage() error = %v, want nil", err)
	}
	if len(cli.pulled) != 1 || cli.pulled[0] != "javdet/nib-agent:v0.8.0" {
		t.Fatalf("pulled = %v, want [javdet/nib-agent:v0.8.0]", cli.pulled)
	}
	if !cli.closed {
		t.Fatal("pull stream was not closed")
	}
}

func TestEnsureImageInspectFailureIsNotAPull(t *testing.T) {
	t.Parallel()

	cli := &fakeImageClient{inspectErr: errors.New("Cannot connect to the Docker daemon")}
	err := ensureImage(context.Background(), cli, "javdet/nib-agent:v0.8.0")
	if !errors.Is(err, ErrImagePull) {
		t.Fatalf("ensureImage() error = %v, want ErrImagePull", err)
	}
	if len(cli.pulled) != 0 {
		t.Fatalf("pulled = %v, want no pull", cli.pulled)
	}
}

func TestEnsureImagePullError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		cli      *fakeImageClient
		wantText string
	}{
		{
			name: "call fails",
			cli: &fakeImageClient{
				inspectErr: errdefs.NotFound(errors.New("No such image")),
				pullErr:    errors.New("pull access denied"),
			},
			wantText: "pull access denied",
		},
		{
			name: "stream reports errorDetail",
			cli: &fakeImageClient{
				inspectErr: errdefs.NotFound(errors.New("No such image")),
				pullBody:   `{"status":"Pulling"}` + "\n" + `{"errorDetail":{"message":"manifest for javdet/nib-agent:v0.8.0 not found"},"error":"manifest unknown"}`,
			},
			wantText: "manifest for javdet/nib-agent:v0.8.0 not found",
		},
		{
			name: "stream reports bare error",
			cli: &fakeImageClient{
				inspectErr: errdefs.NotFound(errors.New("No such image")),
				pullBody:   `{"error":"toomanyrequests"}`,
			},
			wantText: "toomanyrequests",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := ensureImage(context.Background(), tt.cli, "javdet/nib-agent:v0.8.0")
			if !errors.Is(err, ErrImagePull) {
				t.Fatalf("ensureImage() error = %v, want ErrImagePull", err)
			}
			if !strings.Contains(err.Error(), tt.wantText) {
				t.Fatalf("ensureImage() error = %q, want it to contain %q", err, tt.wantText)
			}
			if !strings.Contains(err.Error(), "javdet/nib-agent:v0.8.0") {
				t.Fatalf("ensureImage() error = %q, want it to name the image", err)
			}
		})
	}
}
