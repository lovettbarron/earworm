package audible

import "context"

// Download is stubbed for Phase 4. Returns ErrNotImplemented.
func (c *client) Download(ctx context.Context, asin string, outputDir string) error {
	return ErrNotImplemented
}
