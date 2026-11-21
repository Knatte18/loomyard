// recordslayout.go names the one-repository hub layout and reads it from a store's fetch refspecs.
// It is the only place the layout is spelled:
// the namespace constants, the board branch and the prime's records branch key.

package fabricengine

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/lymdirs"
)

// The one-repository layout keeps a hub's records store under origin's refs/lym/ namespace.
const (
	// RecordsNamespaceRoot is the origin ref prefix that holds the records store's branches and tags.
	RecordsNamespaceRoot = "refs/lym/"
	// RecordsNamespaceHeads is the part of the namespace under RecordsNamespaceRoot that holds branches.
	RecordsNamespaceHeads = "heads/"
	// RecordsNamespaceTags is the part of the namespace under RecordsNamespaceRoot that holds tags.
	RecordsNamespaceTags = "tags/"
	// OneRepositoryBoardBranch is the fixed branch of the board in a one-repository hub.
	// It is a branch lym owns inside its records namespace, never read from git.
	OneRepositoryBoardBranch = "main"
	// PrimeRecordsBranchKey is the records store's git config key that names the prime's records branch.
	PrimeRecordsBranchKey = "lym.primeRecordsBranch"
)

// oneRepositoryNamespace returns the ref namespace of a one-repository hub's records store.
func oneRepositoryNamespace() gitrepo.RefNamespace {
	return gitrepo.RefNamespace{
		Heads: RecordsNamespaceRoot + RecordsNamespaceHeads,
		Tags:  RecordsNamespaceRoot + RecordsNamespaceTags,
	}
}

// namespaceAnswers caches originNamespaceAt's answers for the process, keyed by the cleaned store path.
var namespaceAnswers sync.Map

// originNamespaceAt returns the remote ref namespace of the repository whose worktree is at dir.
// It reads the namespace from the repository's origin fetch refspecs.
// A refspec whose source starts under the records namespace's branches answers the one-repository namespace.
// Any other store, a code store with git's default refspecs included, answers the identity namespace.
// A dir that does not exist or holds no .git entry answers identity and is not cached,
// so a store created there later is read then.
// A repository with no origin remote answers identity, and a read of an existing store is cached for the process.
// A failed config read is returned as an error and not cached.
func originNamespaceAt(dir string) (gitrepo.RefNamespace, error) {
	key := filepath.Clean(dir)
	if cached, found := namespaceAnswers.Load(key); found {
		return cached.(gitrepo.RefNamespace), nil
	}
	if _, err := os.Lstat(filepath.Join(key, ".git")); errors.Is(err, os.ErrNotExist) {
		return gitrepo.RefNamespace{}, nil
	} else if err != nil {
		return gitrepo.RefNamespace{}, fmt.Errorf("fabricengine: look for the repository at %s: %w", key, err)
	}

	refspecs, err := gitrepo.New(key).RemoteFetchRefspecs(originRemoteName)
	if err != nil && !errors.Is(err, gitrepo.ErrRemoteNotConfigured) {
		return gitrepo.RefNamespace{}, fmt.Errorf("fabricengine: read the layout of the repository at %s: %w", key, err)
	}

	answer := gitrepo.RefNamespace{}
	for _, refspec := range refspecs {
		source, _, _ := strings.Cut(strings.TrimPrefix(refspec, "+"), ":")
		if strings.HasPrefix(source, RecordsNamespaceRoot+RecordsNamespaceHeads) {
			answer = oneRepositoryNamespace()
			break
		}
	}
	namespaceAnswers.Store(key, answer)
	return answer, nil
}

// OriginRefNamesAt returns the origin ref names of branch and of the tag under the layout of the repository at dir.
// A one-repository records store keeps both under its refs/lym/ namespace;
// any other repository keeps them under refs/heads/ and refs/tags/.
// It serves a command a person runs by hand, which must name the ref the store's origin holds.
func OriginRefNamesAt(dir, branch, tag string) (branchRef, tagRef string, err error) {
	ns, err := originNamespaceAt(dir)
	if err != nil {
		return "", "", err
	}
	return ns.Branch(branch), ns.Tag(tag), nil
}

// legacyBoltBranch is the branch an earlier one-repository layout kept its board on.
// It is a branch lym owned in that layout, never read from git.
// A clone reads it only to recognise the layout and refuse it.
const legacyBoltBranch = "lym-bolt"

// originLayout is what a one-URL clone reads from origin before it creates anything.
type originLayout struct {
	// defaultBranch is the branch origin's HEAD names, empty when origin reports none.
	defaultBranch string
	// boardTip is the tip of the board's branch under the records namespace, empty when origin holds none.
	boardTip string
	// heads maps each branch under the records namespace, by its local name, to its tip.
	heads map[string]string
	// legacyBolt reports whether origin holds the board branch of the earlier one-repository layout.
	legacyBolt bool
}

// readOriginLayout reads origin's default branch, its records-namespace branches and the earlier layout's board branch.
// It runs one ls-remote against url from cwd, so a clone learns the layout before it creates anything.
// An unreachable origin fails with the probe error prefix, and an unborn origin reads as an empty layout.
func readOriginLayout(cwd, url string) (originLayout, error) {
	ns := oneRepositoryNamespace()
	legacyRef := "refs/heads/" + legacyBoltBranch
	out, err := gitexec.Run([]string{"ls-remote", "--symref", filepath.ToSlash(url), "HEAD", ns.Branch("*"), legacyRef}, cwd)
	if err != nil {
		return originLayout{}, wrapProbeError(url, "ls-remote", err)
	}

	layout := originLayout{heads: map[string]string{}}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if target, isSymref := strings.CutPrefix(line, "ref: refs/heads/"); isSymref {
			layout.defaultBranch, _, _ = strings.Cut(target, "\t")
			continue
		}
		sha, ref, found := strings.Cut(line, "\t")
		if !found {
			continue
		}
		if ref == legacyRef {
			layout.legacyBolt = true
			continue
		}
		local, mapped := ns.LocalRef(ref)
		name, isBranch := strings.CutPrefix(local, "refs/heads/")
		if mapped && isBranch {
			layout.heads[name] = sha
		}
	}
	layout.boardTip = layout.heads[OneRepositoryBoardBranch]
	return layout, nil
}

// refuseOneURLClone refuses a one-URL clone that origin's layout rules out, each refusal naming its way forward.
// A board branch of the earlier one-repository layout is refused with or without opts.SingleRepo.
// With opts.SingleRepo it also refuses an origin that already holds a board.
// It refuses one whose default branch is a records repository.
// It refuses one whose default branch tracks the earlier layout's directories.
// The default branch's root is read through a throwaway shallow clone under cwd.
func refuseOneURLClone(cwd string, layout originLayout, opts CloneOptions) error {
	legacyRefusal := fmt.Errorf("%s holds the branch %s, the layout of an earlier one-repository hub; "+
		"way forward: delete %s on origin and remove %s and %s from the code branches, then clone again",
		opts.WeftURL, legacyBoltBranch, legacyBoltBranch, lymcwd.AnchorFileName, lymdirs.LymDirName)
	if layout.legacyBolt {
		return legacyRefusal
	}
	if !opts.SingleRepo {
		return nil
	}
	if layout.boardTip != "" {
		return fmt.Errorf("%s already holds a one-repository hub's board under %s; way forward: run the same clone without --single-repo to join it",
			opts.WeftURL, RecordsNamespaceRoot)
	}

	bindingFound, recordsFound, err := probeCodeOriginRoot(cwd, opts.WeftURL)
	if err != nil {
		return err
	}
	if bindingFound {
		return fmt.Errorf("%s holds %s on its default branch %s, so it is a records repository; "+
			"way forward: make a two-repository hub with `lym fabric clone <records-url> <code-url>`",
			opts.WeftURL, WarpBindingFileName, layout.defaultBranch)
	}
	if recordsFound {
		return legacyRefusal
	}
	return nil
}
