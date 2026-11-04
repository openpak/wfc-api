package api

import (
	"context"
	"errors"
	"sync"

	"openpak/wfc-common/common"
	"openpak/wfc-common/mkwboard"
)

var (
	eraseLinksOnce sync.Once
	eraseLinks     userLister
	eraseLinksErr  error
)

type userLister interface {
	UserIDs(ctx context.Context, accountID string) ([]int64, error)
}

type userEraser interface {
	EraseUsers(ctx context.Context, userIDs []int64) (int, error)
}

// Erase removes every WFC row of an account (card openpak/prds#120). All WFC services share
// this database, so one call covers nas, gpcm, gpsp, gamestats, sake, race and dls. The
// console ids come from the account core's "wfc" links, which still exist while the website
// finaliser runs its steps.
func Erase(ctx context.Context, accountID string) (int, error) {
	eraseLinksOnce.Do(func() {
		cfg := common.GetConfig()
		if cfg.CoreAddress == "" {
			eraseLinksErr = errors.New("WFC_CORE_ADDRESS not set")
			return
		}
		eraseLinks, _, eraseLinksErr = mkwboard.DialCore(cfg.CoreAddress, cfg.CoreKey)
	})
	if eraseLinksErr != nil {
		return 0, eraseLinksErr
	}
	return eraseAccount(ctx, accountID, eraseLinks, &db)
}

func eraseAccount(ctx context.Context, accountID string, links userLister, e userEraser) (int, error) {
	ids, err := links.UserIDs(ctx, accountID)
	if err != nil {
		return 0, err
	}
	return e.EraseUsers(ctx, ids)
}
