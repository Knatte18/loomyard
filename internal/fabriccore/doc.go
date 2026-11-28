// Package fabriccore holds fabric's rules, as pure functions over what fabric has read.
// A rule's input is the snapshot, the facts read from the hub and from origin,
// and its output is an answer that refuses, asks for another read phase, or lists actions to run.
// The words are "snapshot" and "action list"; neither is called a plan.
//
// The package holds the snapshot, phase, action list and outcome types every rule reads and returns,
// and it is the home of every fabric rule over them.
//
// A snapshot also says which required seams the process has supplied.
// One rule refuses a run that would use a seam that is not supplied.
//
// The failed-push record's rules are here too.
// They merge a new failure and word its notice and way forward.
// They also decide which records leave a push owed or refused.
//
// So are the rules of a side's push.
// They decide the transport retry, the lookup of the branch on origin and the retry of a rejection.
// They also class a failure and word the refusal of a board that was never seeded onto origin.
//
// So are the rules of a pair's origin record and of its beam mark.
// They decide whether the record is marked as a beam's and whether another pair's record is read.
// They also decide how far a read of the commits only origin's branch holds goes.
//
// So are the rules of a pair's push.
// They push the records side again until it lands and class a side that did not.
// They also judge from a branch's local refs whether it is ahead of origin or held there.
//
// So are the rules of judging local branches against origin.
// They decide what is read, when origin is read again and which heads are fetched.
// They also give the verdict on each branch and each entry, and the repair step of a stopped branch.
//
// So are the rules of moving the judged branches.
// They order the moves, say whether each branch moves and judge a records branch once its locks were asked for.
//
// So are the rules of removing the pairs of runs origin has ended.
// They decide which pair is torn down next and what each ended entry reports.
// They also decide which branches and failed-push records go with a pair that is gone.
//
// So are the rules of the records push lock.
// They word its holder for a busy report and decide when the holder loop goes round again.
//
// It imports only the standard library and internal/fabriclayout.
// It runs no git, starts no process, touches no filesystem and takes no lock.
// What reads a snapshot's facts and runs an action list lives in internal/fabricengine.
package fabriccore
