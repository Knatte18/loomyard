# PATTERN-single-repo-hub

Running a hub as one repository: a code store and a records store of one origin, records and the board under origin's `refs/lym/`, mapped by the records store's refspecs; only fabric reads the layout.

## The layout

Rule: A one-repository hub keeps a two-repository hub's local geometry, with both stores cloned from one origin; the records store's branches and tags live under origin's `refs/lym/heads/` and `refs/lym/tags/`, and the board is on the fixed branch `main` there.

- The hub holds a code store and a records store.
  Both fetch from one origin, so origin's branch and tag lists show the code alone.
- The namespace is spelled once, by `fabriclayout`'s `RecordsNamespaceRoot`, `RecordsNamespaceHeads` and `RecordsNamespaceTags`.
  The board's branch is `fabriclayout`'s `OneRepositoryBoardBranch`, never read from git, so a renamed code default cannot hide the board.
- The prime's records branch is not the code store's default branch.
  It is the value of the records store's `lym.primeRecordsBranch` key, which the clone writes and no checkout moves.
  `TestCleanup_ProtectsPrimaryWeftBranchAfterCheckout` and `TestCleanup_RefusesWhenPrimaryWeftBranchIsUndeterminable` hold it over both layouts.
- The records store is configured before any fetch:
  fetch `+refs/lym/heads/*:refs/remotes/origin/*` and `+refs/lym/tags/*:refs/tags/*`,
  push `refs/heads/*:refs/lym/heads/*` and `refs/tags/*:refs/lym/tags/*`, and `remote.origin.tagOpt` `--no-tags`.
  `--no-tags` keeps a code tag out of the store, while the tag refspec still brings the namespaced tags.
- A namespaced records branch keeps `branch.<b>.merge` at its `refs/lym/heads/<b>`, so git's own `@{u}` reads it.
  `gitrepo` reads repository config through one tolerant decode, because go-git refuses a merge setting outside `refs/heads/`.
  `TestUpstreamSHA` and `TestRemoteFetchRefspecs` hold the refspec-following upstream reads.

## The layout is the refspec, read only by fabric

Rule: The records store's fetch refspec is the layout; `gitrepo` learns no layout and fabric alone reads it.

- Fabric's `originNamespaceAt` reads the namespace of the repository at a directory from its `origin` fetch refspecs, and caches it.
  A code store keeps git's default refspecs and reads as the identity namespace of a two-repository hub.
  Helpers that serve both stores read at their own directory and take no side parameter.
- `TestOriginNamespaceAt` holds the read.
- No other package reads the refspecs, spells `refs/lym/`, or branches on the layout.

## The namespace reaches every remote operation

Rule: Every records-store remote operation in `gitrepo` and fabric takes the namespace; no bare push stands.

- `gitrepo.RefNamespace` has origin-side `Heads` and `Tags` prefixes, and its zero value is the identity.
  Every `gitrepo` operation that sends or writes an origin ref name takes it:
  push, lease, push result, upstream write and remote delete.
  `TestRefNamespace` holds the mapping.
- Fabric hands the namespace to its push sites, its ls-remote listings and its fetch sources.
  A code-store or two-repository call passes the zero value, so its behavior is unchanged.
- `TestOneRepositoryPair_RecordsLiveInTheNamespace` holds a pair's records in the namespace and none on a code branch.
  `TestOneRepositoryOrigin_ReconcileAndCleanupSeeTheNamespace` holds the origin reads.
  `TestOneRepositorySecondMachine_AdoptPullAndReplay` holds adopt, pull and the replays from a second machine.
  `TestOneRepositoryMerge_LeavesNoRecordsOnTheCodeBranch` holds the merge.

## The clone

Rule: `CloneHub` takes the one-URL form only after its check and refusals, and seeds the namespace by a leased, ordered push.

- A clone from one URL joins a hub whose origin already holds `refs/lym/`, and `--single-repo` seeds one from a code repository.
- The check refuses an origin that holds the retired layout, naming its fixed `lym-bolt` branch,
  and every refusal names the way forward.
  Fabric defines that name and the board branch once each.
- The seed pushes the records branch first and the board last, each leased on the tip observed before.
  A failed seed removes what it pushed and leaves nothing behind.
- `TestCloneHub_OneURLRefusals` holds the refusals.
  `TestCloneSingleRepo_SeedsTheNamespace` holds the seed and its order.
  `TestCloneOneURL_JoinsTheNamespacedHub` holds the join.
  `TestCloneSingleRepo_FailedSeedPushLeavesNothing` holds the rollback.

## The pair lifecycle runs the two-repository paths

Rule: Add, push, pull, hand-over, remove, cleanup, reconcile and the owner archives run the same code in both layouts, differing only by the namespace they are handed.

- A one-repository hub adds no lifecycle branch: a pair's records branch, its archive tag and its origin delete go through the namespaced operations above.
- The tests above run each verb over `hubforge`'s one-repository builder, and the cleanup tests run as a table over both builders.
