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
	// RecordsNamespaceCodeTags is the part under RecordsNamespaceRoot that holds the code store's archive tags.
	// No fetch refspec of either store maps it, so the records store never lists or fetches a code commit through it.
	RecordsNamespaceCodeTags = "code-tags/"
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

// ErrRecordsLayout is matched by the error of a one-repository records store whose origin layout cannot be read.
// The error's message names the git config that restores the layout.
var ErrRecordsLayout = errors.New("the records store's origin layout cannot be read")

// namespaceAnswers caches originNamespaceAt's answers for the process, keyed by the cleaned store path.
var namespaceAnswers sync.Map

// originNamespaceAt returns the remote ref namespace of the repository whose worktree is at dir.
// It reads the namespace from the repository's origin fetch refspecs.
// A refspec whose source starts under the records namespace's branches answers the one-repository namespace.
// Any other store, a code store with git's default refspecs included, answers the identity namespace.
// A dir that does not exist or holds no .git entry answers identity and is not cached,
// so a store created there later is read then.
// A repository with no origin remote answers identity, and a read of an existing store is cached for the process.
// A store that holds the prime's records branch key or a push refspec into the namespace is a records store.
// So is one with a branch whose merge setting names a ref under the namespace.
// For such a store a missing namespaced fetch refspec or a missing origin is an error naming the way forward:
// read as identity, its board would push to origin's code branches.
// That error matches ErrRecordsLayout.
// A failed config read is returned as an error, and no error is cached.
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
	originMissing := errors.Is(err, gitrepo.ErrRemoteNotConfigured)
	if err != nil && !originMissing {
		return gitrepo.RefNamespace{}, fmt.Errorf("fabricengine: read the layout of the repository at %s: %w", key, err)
	}

	ns := oneRepositoryNamespace()
	answer := gitrepo.RefNamespace{}
	for _, refspec := range refspecs {
		source, _, _ := strings.Cut(strings.TrimPrefix(refspec, "+"), ":")
		if strings.HasPrefix(source, ns.Heads) {
			answer = ns
			break
		}
	}
	if answer == (gitrepo.RefNamespace{}) {
		claim, err := recordsStoreClaim(key)
		if err != nil {
			return gitrepo.RefNamespace{}, fmt.Errorf("fabricengine: read the layout of the repository at %s: %w", key, err)
		}
		if claim != "" {
			restoreFetch := fmt.Sprintf("`git -C %s config --replace-all remote.%s.fetch '+%s*:refs/remotes/%s/*'` and `git -C %s config --add remote.%s.fetch '+%s*:refs/tags/*'`",
				key, originRemoteName, ns.Heads, originRemoteName, key, originRemoteName, ns.Tags)
			layoutMessage := fmt.Sprintf("fabricengine: the repository at %s holds %s, so it is a one-repository records store, "+
				"but no %s fetch refspec maps %s; way forward: restore its fetch refspecs with %s",
				key, claim, originRemoteName, ns.Heads, restoreFetch)
			if originMissing {
				layoutMessage = fmt.Sprintf("fabricengine: the repository at %s holds %s, so it is a one-repository records store, but it has no %s remote; "+
					"way forward: add it with `git -C %s remote add --no-tags %s <code-url>`, restore its fetch refspecs with %s, "+
					"and its push refspecs `refs/heads/*:%s*` and `refs/tags/*:%s*`",
					key, claim, originRemoteName, key, originRemoteName, restoreFetch, ns.Heads, ns.Tags)
			}
			return gitrepo.RefNamespace{}, &sentinelError{sentinel: ErrRecordsLayout, msg: layoutMessage}
		}
	}
	namespaceAnswers.Store(key, answer)
	return answer, nil
}

// recordsStoreClaim names the setting that marks the repository at dir a one-repository records store.
// It returns empty when no setting does.
// A non-empty prime's records branch key marks it, and so does an origin push refspec into the records namespace.
// A branch whose merge setting names a ref under that namespace marks it too.
func recordsStoreClaim(dir string) (string, error) {
	pushKey := "remote." + originRemoteName + ".push"
	pattern := "^(" + strings.ReplaceAll(pushKey, ".", `\.`) + "|" + strings.ReplaceAll(strings.ToLower(PrimeRecordsBranchKey), ".", `\.`) + `|branch\..+\.merge)$`
	listing, err := gitexec.Run([]string{"config", "--get-regexp", pattern}, dir)
	var gitErr *gitexec.GitError
	if errors.As(err, &gitErr) && gitErr.ExitCode == 1 {
		// git config exits 1 when no key matches.
		return "", nil
	}
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(listing, "\n") {
		name, value, _ := strings.Cut(strings.TrimSpace(line), " ")
		value = strings.TrimSpace(value)
		switch {
		case name == strings.ToLower(PrimeRecordsBranchKey) && value != "":
			return fmt.Sprintf("%s %s", PrimeRecordsBranchKey, value), nil
		case name == pushKey && strings.Contains(value, RecordsNamespaceRoot):
			return fmt.Sprintf("the push refspec %s", value), nil
		case strings.HasPrefix(name, "branch.") && strings.HasSuffix(name, ".merge") && strings.HasPrefix(value, RecordsNamespaceRoot):
			return fmt.Sprintf("the upstream setting %s %s", name, value), nil
		}
	}
	return "", nil
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
// Without opts.SingleRepo it refuses an origin that holds records branches and no board:
// a seed that stopped before its board landed leaves that, and the way forward is the seed.
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
		if len(layout.heads) > 0 && layout.boardTip == "" {
			return fmt.Errorf("%s holds records under %s and no board, so a one-repository seed stopped before its board landed; "+
				"way forward: run `lym fabric clone --single-repo --shortname <shortname> %s` to seed it",
				opts.WeftURL, RecordsNamespaceRoot, opts.WeftURL)
		}
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
