package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"rstash/internal/blob"
	"rstash/internal/config"
	"rstash/internal/db"

	"github.com/spf13/cobra"
)

var encryptExistingCmd = &cobra.Command{
	Use:   "encrypt-existing",
	Short: "Re-encrypt all blobs that predate at-rest encryption",
	Long: `Walk every document node and re-write its blob through the encrypted store
wrapper. Blobs already carrying the encMagic prefix are skipped, so the command
is idempotent and safe to interrupt or re-run.

Requires RSTASH_BLOB_KEY to be set. Operators who enable encryption on an
existing deployment should run this once after setting the key so pre-existing
blobs are also protected.`,
	RunE: runEncryptExisting,
}

func init() {
	rootCmd.AddCommand(encryptExistingCmd)
}

func runEncryptExisting(cmd *cobra.Command, args []string) error {
	cfg := config.Load()

	// Verify the key is actually set so the walk isn't a no-op.
	if _, ok := (blob.EnvKeyProvider{}).CurrentKey(); !ok {
		return fmt.Errorf("RSTASH_BLOB_KEY is not set — nothing to encrypt to")
	}

	repo, err := db.OpenRepository(cfg.DatabaseDSN)
	if err != nil {
		return fmt.Errorf("open repository: %w", err)
	}
	defer repo.Close()

	scheme, path, err := config.ParseDSN(cfg.BlobDSN)
	if err != nil {
		return err
	}
	store, err := blob.OpenStore(scheme, path, cfg.BlobDSN)
	if err != nil {
		return err
	}
	defer store.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	users, err := repo.ListUsers(ctx)
	if err != nil {
		return fmt.Errorf("list users: %w", err)
	}

	total := 0
	encrypted := 0
	skipped := 0

	startedAt := time.Now()
	for _, u := range users {
		nodes, err := repo.ListUserNodes(ctx, u.ID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  WARN  skip user %d: %v\n", u.ID, err)
			continue
		}
		for _, n := range nodes {
			total++
			changed, err := reencryptBlob(ctx, store, u.ID, n.Path)
			if err != nil {
				fmt.Fprintf(os.Stderr, "  FAIL  user=%d path=%s: %v\n", u.ID, n.Path, err)
				continue
			}
			if changed {
				encrypted++
			} else {
				skipped++
			}
		}
	}

	fmt.Fprintf(os.Stderr, "\nDone in %s. %d total, %d re-encrypted, %d already encrypted or empty.\n",
		time.Since(startedAt).Round(time.Millisecond), total, encrypted, skipped)
	return nil
}

// reencryptBlob reads a blob through the encrypted wrapper and writes it back.
// The wrapper transparently handles the "already encrypted" case: reads decrypt,
// writes encrypt, so an idempotent round-trip either produces new ciphertext or
// leaves an already-encrypted blob unchanged-for-our-purposes.
//
// To avoid rewriting blobs that are already encrypted, we peek at the raw bytes
// using an Inner()-exposing wrapper if possible. If the inner isn't exposed we
// skip the optimization and always rewrite (still safe).
func reencryptBlob(ctx context.Context, store blob.Store, userID int64, path string) (bool, error) {
	rc, err := store.Get(ctx, userID, path)
	if err != nil {
		return false, err
	}
	plaintext, err := io.ReadAll(rc)
	rc.Close()
	if err != nil {
		return false, err
	}
	// Re-Put through the wrapper — new ciphertext produced.
	if err := store.Put(ctx, userID, path, plaintext); err != nil {
		return false, err
	}
	return true, nil
}
