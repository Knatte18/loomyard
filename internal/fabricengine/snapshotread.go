// snapshotread.go holds the phased snapshot reader, which fills the fields a phase names from the hub and its stores.

package fabricengine

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/dotgit"
	"github.com/Knatte18/loomyard/internal/fabriccore"
	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/gitrepo"
)

// snapshotReader reads the fields of a snapshot from one hub.
type snapshotReader struct {
	// hub is the hub's path; without one no field of the hub itself is read.
	hub string
	// stores maps each store the reader holds to its repository directory.
	stores map[fabriccore.Store]string
}

// read fills the named fields of snapshot and leaves the others as they are, whichever the phase.
// A field no store or hub path of the reader answers stays empty.
// The records store's layout is cached for the process, so a later read of that field answers as the first did.
// A failed field or store stops no other, and what failed stays as it was.
// The error joins one *fieldReadError for each failure, in read order.
func (r snapshotReader) read(phase fabriccore.Phase, fields []fabriccore.Field, snapshot *fabriccore.Snapshot) error {
	var failures []error
	fail := func(field fabriccore.Field, store fabriccore.Store, err error) {
		failures = append(failures, &fieldReadError{Field: field, Store: store, Err: err})
	}
	eachStore := func(field fabriccore.Field, readStore func(store fabriccore.Store, dir string, refs *fabriccore.StoreRefs) error) {
		for _, store := range []fabriccore.Store{fabriccore.StoreCode, fabriccore.StoreRecords, fabriccore.StoreBolt} {
			dir, held := r.stores[store]
			if !held {
				continue
			}
			refs := snapshot.Stores[store]
			if err := readStore(store, dir, &refs); err != nil {
				fail(field, store, err)
				continue
			}
			if snapshot.Stores == nil {
				snapshot.Stores = map[fabriccore.Store]fabriccore.StoreRefs{}
			}
			snapshot.Stores[store] = refs
		}
	}
	codeDir, holdsCode := r.stores[fabriccore.StoreCode]
	recordsDir, holdsRecords := r.stores[fabriccore.StoreRecords]

	for _, field := range fields {
		switch field {
		case fabriccore.FieldWorktrees:
			if !holdsCode {
				continue
			}
			worktrees, err := List(codeDir)
			if err != nil {
				fail(field, fabriccore.StoreCode, err)
				continue
			}
			snapshot.Worktrees = worktrees

		case fabriccore.FieldLocalRefs:
			eachStore(field, func(_ fabriccore.Store, dir string, refs *fabriccore.StoreRefs) error {
				local, err := readLocalRefs(dir)
				if err != nil {
					return err
				}
				refs.Local = local
				return nil
			})

		case fabriccore.FieldOriginRefs:
			eachStore(field, func(store fabriccore.Store, dir string, refs *fabriccore.StoreRefs) error {
				heads, err := lsRemoteHeads(string(store), dir, ReconcileOptions{})
				if err != nil {
					return err
				}
				refs.Origin = fabriccore.Refs{}
				for branch, commit := range heads {
					refs.Origin["refs/heads/"+branch] = commit
				}
				refs.OriginRead = true
				return nil
			})

		case fabriccore.FieldAnchors:
			if !holdsRecords {
				continue
			}
			anchors, err := func() (map[string]string, error) {
				worktrees, err := List(recordsDir)
				if err != nil {
					return nil, err
				}
				anchors := map[string]string{}
				for _, worktree := range worktrees {
					if worktree.Prunable {
						continue
					}
					path := filepath.FromSlash(worktree.Path)
					marker, found, err := committedMarker(path, "HEAD")
					if err != nil {
						return nil, err
					}
					if found {
						anchors[path] = marker
					}
				}
				return anchors, nil
			}()
			if err != nil {
				fail(field, fabriccore.StoreRecords, err)
				continue
			}
			snapshot.Anchors = anchors

		case fabriccore.FieldRecordsStore:
			if !holdsRecords {
				continue
			}
			if _, err := os.Stat(recordsDir); errors.Is(err, fs.ErrNotExist) {
				snapshot.RecordsStore = fabriccore.RecordsStore{}
				continue
			} else if err != nil {
				fail(field, fabriccore.StoreRecords, err)
				continue
			}
			namespace, err := originNamespaceAt(recordsDir)
			if err != nil {
				fail(field, fabriccore.StoreRecords, err)
				continue
			}
			snapshot.RecordsStore = fabriccore.RecordsStore{Present: true, Heads: namespace.Heads, Tags: namespace.Tags}

		case fabriccore.FieldPendingTeardowns:
			if r.hub == "" {
				continue
			}
			pending, err := PendingTeardowns(r.hub)
			if err != nil {
				// The read is of the hub alone, so it names no store.
				fail(field, "", err)
				continue
			}
			snapshot.PendingTeardowns = pending
		}
	}
	return errors.Join(failures...)
}

// readLocalRefs returns the branches and tags of the repository whose worktree root is dir, keyed by full ref name.
// An annotated tag maps to its commit, and a tag naming no commit is left out.
// A repository whose ref storage is not files is read through git, and any other in process.
func readLocalRefs(dir string) (fabriccore.Refs, error) {
	refs := fabriccore.Refs{}
	_, err := dotgit.ListWorktrees(dir)
	if errors.Is(err, dotgit.ErrRefStorage) {
		format := "--format=%(refname) %(objecttype) %(objectname) %(*objecttype) %(*objectname)"
		out, err := gitexec.Run([]string{"for-each-ref", format, "refs/heads", "refs/tags"}, dir)
		if err != nil {
			return nil, fmt.Errorf("list the refs of %q: %w", dir, err)
		}
		for _, line := range strings.Split(out, "\n") {
			// An annotated tag adds the type and name of what it peels to.
			parts := strings.Fields(line)
			switch {
			case len(parts) == 3 && parts[1] == "commit":
				refs[parts[0]] = parts[2]
			case len(parts) == 5 && parts[3] == "commit":
				refs[parts[0]] = parts[4]
			}
		}
		return refs, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read the ref storage of %q: %w", dir, err)
	}

	repo := gitrepo.New(dir)
	branches, err := repo.BranchRefs()
	if err != nil {
		return nil, fmt.Errorf("list the branches of %q: %w", dir, err)
	}
	tags, err := repo.TagRefs("")
	if err != nil {
		return nil, fmt.Errorf("list the tags of %q: %w", dir, err)
	}
	for _, branch := range branches {
		refs["refs/heads/"+branch.Name] = branch.Commit
	}
	for _, tag := range tags {
		refs["refs/tags/"+tag.Name] = tag.Commit
	}
	return refs, nil
}
