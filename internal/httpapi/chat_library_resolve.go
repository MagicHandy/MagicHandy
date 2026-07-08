package httpapi

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/mapledaemon/MagicHandy/internal/library"
)

func (s *Server) resolveLibraryBlockByPatternTag(ctx context.Context, tag string) (string, error) {
	if s.library == nil {
		return "", errors.New("pattern library is unavailable")
	}
	normalized := strings.ToLower(strings.TrimSpace(tag))
	if normalized == "" {
		return "", errors.New("padrao_id is required")
	}

	blocks, err := s.library.Store().ListMotionBlocks(ctx, library.BlockFilter{
		HideBlocked: true,
		Limit:       500,
	})
	if err != nil {
		return "", err
	}
	for _, block := range blocks {
		for _, candidate := range block.Tags {
			if strings.EqualFold(candidate, normalized) {
				return block.ID, nil
			}
		}
	}
	return "", fmt.Errorf("no library block found for padrao_id %q", normalized)
}
