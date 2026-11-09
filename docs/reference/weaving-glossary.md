# Weaving glossary

A glossary of weaving and textile terms that can serve as names in lym.
It lists and marks; it recommends no name for any open question.

## How to read it

Each row gives the English term, the Norwegian term, other languages where a term is well known, and a one-line meaning in weaving.
It also gives the kind of lym thing the term would fit and whether lym already holds the name.

- **Fits** is one of: recipe, agent role, module, artifact, state, or verb.
  It names the kind of thing the term's meaning resembles, not a decision to use it.
- **In lym** is `–` when the search below found no hit.
  `Taken:` says what holds the name today, in one or two sentences.
  They come from the package doc, the command's help or the board's label descriptions.
  `Close:` names a taken term the entry could be confused with and says what that term is the same way.
- **Norwegian** terms without a mark appear in the Norwegian sources listed under Sources.
  A term marked `†` was not found in those sources;
  it comes from general Norwegian weaving usage and needs a check against a Norwegian weaving reference before use.
- **Other languages** are Swedish (sv), Danish (da), French (fr) and German (de).
  A `–` means no well-known term was recorded.
  These columns were not checked entry by entry against a dictionary.

## What "taken" was searched against

- Package names under `internal/` and the module names `lym --help` lists.
- The recipe names in `contracts/recipes` and the row names in those recipes.
  Row names include Discussion, Plan, Webster, Describe, Finalize, Land and the Beam rows.
- The `lym` verbs of every module.
- The role constants in Go code and the stencil families under the Bolt stencils directory.
  Their seats are chair, advisor, writer, implementer, reviewer and fixer.
- The board's statuses and layers (draft, running, ready, project, done) and its kinds (task, note, project).
  Its handle letters and the labels from `lym board labels` count too.
- Names that board entries and Go comments reserve:
  - `cartoon`, kept for a later Plan block beside the `motif` recipe (the shed-sub-run-block task).
  - `hardener` and `tenter` (the hardener note).
  - `weaver` (the weaver note).

## Tools and loom parts

| English | Norwegian | Other | Meaning | Fits | In lym |
|---|---|---|---|---|---|
| loom | vevstol, vev | sv vävstol; da væv; fr métier à tisser; de Webstuhl | the frame that holds the warp and weaves it | module | Taken: `loom`, the module (`lym loom`) that drives one task's phase machine over a per-worktree status file, on the generic shed engine. It walks the producer rows from Discussion through Plan and Webster to Finalize. |
| warp | renning, varp | sv varp; da kæde; fr chaîne; de Kette | the lengthwise threads held under tension | module, artifact | Taken: `warp`, the code-repository half of a fabric pair. Every warp worktree has a weft worktree beside it. |
| weft | veft, innslag, islett † | sv inslag; da islæt; fr trame; de Schuss | the crosswise thread woven through the warp | module, artifact | Taken: `weft`, the records-repository half of a fabric pair, kept in the `-weft` sibling directory of each warp worktree. |
| shed | skille, trådskille | sv fack; da fag; fr pas; de Fach | the opening between raised and lowered warp threads | module | Taken: `shed`, the phase-machine layer (`lym shed`) that drives an addressed run through the generic run, step, status, pause and goto verbs. Its recipes live in `contracts/recipes`. |
| shuttle | skyttel | sv skyttel; da skyttel; fr navette; de Schützen, Webschiffchen | carries the weft through the shed | module | Taken: `shuttle`, the module (`lym shuttle`) that drives one LLM agent through a single run: a prompt goes in, and the agent's output files and a classified outcome come out. The provider behind it is swappable. |
| reed | vevskje | sv vävsked; da vevskje †; fr peigne, ros; de Webblatt, Riet | the comb that spaces the warp and beats the weft in | module | Taken: `reed`, the module (`lym reed`) that drives a per-hub tmux server with one strand per pane. It adds, removes, resumes and attaches strands and renders their layout. |
| heddle | hovel | sv solv; da solv; fr lisse; de Litze | the eye on a shaft that lifts one warp thread | module | Taken: `heddle`, the module (`lym heddle`) that delivers the instruction text agents get from two layers: the entries lym ships in its binary and the repository's own `heddle.json` in the Bolt checkout. |
| shaft, harness | skaft | sv skaft; da skaft; fr lame, cadre; de Schaft | a frame of heddles raised or lowered together | module, state | – |
| treadle | tre (pl. trøer) | sv trampa; da trin; fr marche; de Tritt | the foot pedal that moves the shafts on a hand loom | module | Taken: `treadleengine`, the generalized round-loop engine: it spawns a round through a caller-supplied runner, gates convergence and runs a progress judge. It has no consumer today and is kept for the future Tenter module. |
| dobby | skaftmaskin, dobby | sv skaftmaskin; fr ratière; de Schaftmaschine | the machine that drives the shafts from a pattern | agent role, module | Taken: `dobby`, the daemon that drives the batten runs of one beam worktree, one process per beam on its owning machine. |
| jacquard | jacquard | sv jacquard; fr jacquard; de Jacquard | the machine that controls warp threads singly | module, agent role | – |
| cam, eccentric | eksentermaskin | fr came; de Exzenter | the mechanism that moves the shafts of a mechanical loom | module | – |
| warp beam | bom, renningsbom † | sv varpbom; fr ensouple; de Kettbaum | the roller that carries the warp | module, state | Close: `beam`, a long-lived pair (`lym beam`) that groups board work: its tasks fork from the beam's branch and land back into it. A beam is one unit of a dobby and an orch. |
| cloth beam | tøystokk | sv tygbom; fr ensouple de devant; de Warenbaum | the roller that takes up the finished cloth | module, artifact | Close: `beam`, as above. |
| breast beam | brystbom | sv bröstbom; fr poitrinière; de Brustbaum | the beam the cloth passes over at the weaver's chest | module | Close: `beam`, as above. |
| batten | slagbom †, vevslag † | sv slagbom; fr battant; de Weblade | the swinging frame that holds the reed | module | Taken: `batten`, the module (`lym batten`) that drives one task worktree's whole lifecycle (create, seed the task's own run, run it to a terminal state, tear down) as a single shed run. |
| creel | spolerame †, garnstativ † | sv spolram; fr cantre; de Schärgatter | the rack that holds the bobbins while warping | module | Taken: `creel`, the hub's message daemon and its verbs. An agent posts a message for a strand and the strand gets it in full at its next tool call or turn end, through hooks. |
| bobbin, spool | spole | sv spole; da spole; fr bobine; de Spule | holds the weft yarn inside the shuttle | artifact | – |
| spindle | slire, spinnerokk † | sv slända; fr fuseau; de Spindel | the stick that twists fibre into yarn | module | – |
| distaff | rokk | sv rock; da rok; fr quenouille; de Spinnrocken | holds the fibre while spinning | module | – |
| spinning wheel | rokk, spinnerokk † | sv spinnrock; fr rouet; de Spinnrad | turns fibre into yarn | module | – |
| warping board, warping mill | renneramme †, rennebom | sv varpställning; fr ourdissoir; de Schärrahmen | the frame the warp is measured and wound on | module | – |
| tenter, tenter frame | – † | fr rame; de Spannrahmen | the frame that stretches finished cloth to dry | module, agent role | Taken: `Tenter`, a planned module named in the `treadleengine` doc and in the final-summary spec as a future last-content-producing step. The board's `hardener` note describes it with Hardener as an autonomous behavior-review campaign. |
| cloth, fabric | tøy, vevnad | sv tyg; da stof; fr tissu; de Gewebe | the woven result | module, artifact | Taken: `fabric`, the module (`lym fabric`) that keeps a hub's code (warp) and records (weft) git repositories paired. It creates, switches, syncs, merges and tears down both sides of a worktree pair together. |
| bolt of cloth | tøyrull †, nøste (yarn) | fr rouleau; de Ballen | a rolled length of finished cloth | module | Taken: Bolt (`_bolt`), the hub's second records worktree, checked out on the code repository's default branch. It holds the board, the stencils and the specs. |
| selvedge, selvage | jare, jarekant | sv list; da kant; fr lisière; de Webkante | the firm edge that stops the cloth fraying | module, state | Taken: `Selvage`, the always-present console pane of the reed session. It sits outside the strands and carries no strand name or color. |
| fell | vevnadsfronten | fr chaîne tissée; de Gewebekante | the working edge of the cloth where the weft is beaten in | state | – |
| lease sticks | kryssstokker † | sv korsstavar; fr croix; de Kreuzstäbe | hold the crossing of warp threads in order | module | Close: the file locks of the `lock` package, which are file-based advisory locks (an exclusive write lock and a shared read lock) that coordinate processes. |
| tapestry loom, haute lisse, basse lisse | oppstadgogn, flatvevstol | fr haute lisse, basse lisse | the upright and the horizontal loom for pictorial weaving | module | Close: `tapestry`, the full recipe (Discussion, Plan, Webster, Code review, Describe, Finalize) loom drives for a task. |
| tension weights | vevtyngder | sv varpvikter; de Kettgewichte | weights that hold the warp taut | module | – |
| comb | kam † | sv kam; fr peigne; de Kamm | straightens and aligns fibre or threads | module | – |

## Processes and actions

| English | Norwegian | Other | Meaning | Fits | In lym |
|---|---|---|---|---|---|
| weaving | veving | sv vävning; da vævning; fr tissage; de Weben | making cloth by crossing warp and weft | recipe, module | – |
| warping | renning, å renne | sv varpning; fr ourdissage; de Schären | measuring and winding the warp onto the beam | recipe, verb | – |
| threading, drafting in | tråing †, inndraging † | sv solvning †; fr rentrage; de Einziehen | passing each warp thread through its heddle and the reed | recipe, verb | – |
| sleying | – † | fr passage au peigne; de Einlesen | passing the warp through the reed | verb | – |
| tying up | oppbinding † | fr attache | tying the treadles to the shafts | verb | – |
| picking, inserting weft | innlegging | fr passée; de Schusseintrag | sending the shuttle through the shed once | verb | – |
| beating | slå inn | fr frappe; de Anschlagen | pushing the weft against the fell with the reed | verb | – |
| shedding | å åpne skillet † | fr ouvrir le pas | opening the shed | verb | Close: `shed`, the phase-machine layer described above. |
| spinning | spinning, å spinne | sv spinning; fr filage; de Spinnen | twisting fibre into yarn | recipe, verb | – |
| carding | karding † | sv kardning; fr cardage; de Kardieren | combing fibre so it lies parallel | recipe, verb | Close: a plan card, one numbered unit of a plan that webster groups into batches and forks an implementer for. |
| dyeing | farging | sv färgning; fr teinture; de Färben | colouring yarn or cloth | recipe, verb | – |
| fulling, waulking | valking, å valke | sv valkning; fr foulage; de Walken | shrinking and thickening cloth with moisture and pressure | recipe, state | – |
| finishing | etterbehandling † | fr apprêt; de Appreteur | the last treatment of woven cloth | recipe, state | – |
| burling | – † | fr époutissage; de Noppen | picking knots and slubs from cloth before finishing | recipe | Taken: `burler`, the module (`lym burler`) that drives one review-and-fix round over an artifact with two sessions run one after the other. Its package doc says it is named for burling and mending. |
| mending | å lappe †, å reparere | fr remaillage | repairing defects in woven cloth | recipe, verb | Taken: the fix half of a `burler` round, which repairs the findings of the review half. |
| darning | stopping, å stoppe | sv stoppning; da stoppe; fr reprisage; de Stopfen | weaving new thread across a hole | recipe | Taken: `darn`, the recipe that makes the change a board entry asks for in one writer session, then verifies, reviews and lands it. Its hub-wide configuration is `darn.yaml`. |
| tenterhooking | – † | fr ramage | stretching cloth on tenter hooks | verb | Close: `Tenter`, the planned module described above. |
| tying a knot | å knyte | fr nouer | joining two thread ends | verb | – |
| unweaving, raveling | å rekke opp | fr défaire | taking a woven piece apart thread by thread | verb | – |

## Agent nouns

Agent nouns name the one who does the work.

| English | Norwegian | Other | Meaning | Fits | In lym |
|---|---|---|---|---|---|
| weaver | vever, veverske | sv vävare; da væver; fr tisserand; de Weber | works the loom | agent role | Taken: `Weaver`, a board note (F13) for a cold-start implementer beside Webster, a fresh agent per batch that escalates its model when an agent fails. Webster itself is the fork-based implementer loop. |
| warper | renner † | sv varpare †; fr ourdisseur; de Schärer | prepares the warp | agent role | – |
| drafter | mønstertegner † | sv mönsterritare †; fr dessinateur; de Musterzeichner | draws the pattern draft | agent role | Close: `draft`, the board status of a task that is not released: every new task takes it, bar a task a write releases, such as the raddle promotion, and a draft entry holds no run lock. |
| threader | – † | fr rentreur | draws the warp through heddles and reed | agent role | – |
| spinner | spinner, spinnerske | sv spinnare; fr fileur; de Spinner | spins yarn | agent role | – |
| carder | – † | fr cardeur; de Kardierer | cards fibre | agent role | Close: a plan `card`, as above. |
| dyer | farger | sv färgare; fr teinturier; de Färber | dyes yarn or cloth | agent role | – |
| fuller, waulker | valker | sv valkare; fr foulon; de Walker | fulls cloth | agent role | – |
| shearer, cropper | – † | fr tondeur; de Tuchscherer | trims the nap from finished cloth | agent role | – |
| burler | – † | fr épinceteuse | inspects cloth for defects and picks them out | agent role | Taken: `burler`, the review-and-fix round module described above. Its reviewer role is `burler-review`. |
| mender, darner | stopper † | fr stoppeuse, repriseuse | repairs holes and defects | agent role | Close: `darn`, the recipe described above. |
| sampler (maker of samples) | – † | – | weaves a sample to test a design | agent role | Taken: `sampler`, a recipe and an agent role. Its hub-wide config `sampler.yaml` holds the model-spec of the sampler agent's interactive session and whether the Code-Review perch runs on a sampler run. |
| tenter | – † | fr rameur | stretches cloth on the tenter frame | agent role | Taken: `Tenter`, the planned module described above. |
| finisher | etterbehandler † | fr apprêteur; de Appreteur | applies the finishing steps | agent role | – |
| tackler | – † | – | the overseer of looms in a mill; also the one who rigs a loom | agent role | Taken: `tackler`, the cold agent the shed driver asks about a stop that no rule in its closed rule table settles. It is the second stage of a shed run's stop chain. |
| loom fixer, tuner | – † | – | keeps mill looms running | agent role | Close: `fixer`, the seat that repairs the findings of a burler or landing review. |
| driver | – | – | not a weaving term | – | Taken: `driver`, the session that drives one seeded shed run through the detached `lym shed step --until-stop` loop. |

## Structures and weaves

| English | Norwegian | Other | Meaning | Fits | In lym |
|---|---|---|---|---|---|
| weave, binding | binding | sv bindning; fr armure; de Bindung | the crossing pattern of warp and weft | state, module | – |
| plain weave, tabby | lerretsbinding, toskaftbinding | sv tuskaft; fr toile; de Leinwandbindung | over one, under one | state | – |
| twill | kypert, kypertbinding | sv kypert; da kipper; fr sergé; de Köper | diagonal ribs from a stepped binding | module, recipe | Taken: `twill`, the directive (rendered by `twilldirective`) that makes a writer which has settled its record review it through lenses, one fork per lens, before it ends. |
| satin | sateng, satengbinding | sv satäng; fr satin; de Atlas | long floats, smooth surface | recipe, state | – |
| damask | damask | sv damast; fr damas; de Damast | figured weave with a pattern in contrasting binding | artifact | – |
| jacquard weave | jacquard | fr jacquard | pictorial weave controlled thread by thread | artifact | – |
| pile | flor †, polvarp (pile warp) | sv lugg; fr velours; de Flor | raised loops or cut ends on the surface | state | – |
| terry | frotté | fr éponge; de Frottee | pile of uncut loops | state | – |
| tapestry | billedvev, gobelenger (pl.) | sv gobelängvävnad; fr tapisserie; de Gobelin | weft-faced pictorial weave | recipe, artifact | Taken: `tapestry`, the full task recipe: Discussion, Plan, Webster, Code review, Describe and Finalize, each review step followed by its own fix step. |
| arras | – | fr arras | a tapestry hung as a wall covering | recipe | Taken: `arras`, the recipe that discusses a task and splits its decision record into board entries under the split perch's review. |
| overshot | – † | sv opphämta †; de Überschuss | pattern from long weft floats on a plain ground | state | – |
| rep, ribbed weave | rips † | fr reps; de Rips | weave with fine crosswise ribs | state | – |
| double cloth | dobbeltvev | fr double étoffe; de Doppelgewebe | two layers woven at once | state, recipe | – |
| rapport, repeat | rapport | sv rapport; fr rapport; de Rapport | the smallest unit that repeats in a pattern | artifact | – |
| diaper | dreielsmønster | fr losange | a small repeated geometric pattern | state | – |
| float | flott † | fr flotté; de Flottierung | a thread that passes over several crossing threads | state | – |
| ground | grunnvev † | fr fond; de Grund | the plain base under a pattern | state | – |
| twill-derived denim | dongeri | fr denim | a twill cloth, warp-faced | state | – |

## Artifacts and stages

| English | Norwegian | Other | Meaning | Fits | In lym |
|---|---|---|---|---|---|
| design | mønster †, utkast † | sv mönster; fr dessin; de Entwurf | the pattern to be woven | artifact | – |
| draft | opptegning † | sv uppritning †; fr mise en carte; de Patrone | the written weaving pattern: threading, tie-up, treadling | artifact | Taken: `draft`, the board status of a task that is not released: every new task takes it, bar a task a write releases, such as the raddle promotion, and a draft entry holds no run lock. The board layer `Draft` lists such tasks. |
| cartoon | kartong | sv kartong; fr carton; de Karton | the full-size drawing placed behind the warp | artifact | Taken: `cartoon`, kept for a later Plan block by the shed-sub-run-block task. |
| sample, swatch | prøve, vevprøve † | sv provlapp; fr échantillon; de Muster, Probe | a small piece woven to test a design | artifact | Close: `sampler`, the recipe and agent role described above. |
| motif | motiv | sv motiv; fr motif; de Motiv | a repeated unit of a design | artifact, recipe | Taken: `motif`, a recipe a board task may name, which runs Discussion alone, and which tapestry runs inline as its Motif block, a shed block that runs a whole sub-recipe as one producer. |
| raddle | renningskam † | sv –; fr râteau; de – | the comb that spaces the warp threads as they are wound onto the beam | artifact, recipe | Taken: `raddle`, a recipe a board task runs on a Sketch, whose agent proposes how the Sketch splits into a tapestry, tasks, a project or a beam, and which files what the operator approves on the board. |
| warp chain | renningskjede † | fr chaîne; de Kettfaden-Kette | the warp as a chain of ends after warping | artifact | – |
| yarn | garn | sv garn; da garn; fr fil; de Garn | spun fibre ready to weave | artifact | – |
| thread | tråd | sv tråd; fr fil; de Faden | a single strand | artifact | Close: `strand`, as below. |
| strand | – | fr brin; de Strang | one ply or fibre bundle | artifact | Taken: `strand`, the unit reed lays out as one tmux pane: one agent session of a worktree's reed session. |
| skein, hank | nøste † , dokke | sv härva; fr écheveau; de Strang | yarn wound in a loose coil | artifact | Close: `strand`, as above. |
| ball of yarn | nøste, garnnøste | sv nystan | yarn wound into a ball | artifact | – |
| bolt | – | fr rouleau | a rolled length of cloth | artifact | Taken: Bolt, the records checkout described above. |
| piece, length | stykke † | fr pièce; de Stück | one finished length of cloth | artifact | – |
| selvedge mark | – | – | a mark woven in the edge to identify a cloth | artifact | Close: `Selvage`, the console pane described above. |
| label, mill mark | merke † | fr marque | identifies the maker | artifact | – |
| note | – | – | not a weaving term | – | Taken: `note`, a board kind: an idea or observation. Unlike a task, it cannot be claimed or depend on other tasks. |
| perch | – | – | a frame on which cloth is inspected | state, artifact | Taken: `perch`, a named review stage of a loom recipe, such as the Code-Review perch with its bouncer, review and fix rows and its `loom.yaml` keys for models, fans and budget. |
| pattern book, record | mønsterbok † | sv mönsterbok; de Musterbuch | a bound collection of drafts | artifact | Close: `PATTERN.md` and the `pattern` package, which hold the repository's cross-cutting rules and the directive text agents read about them. |

## Defects and repairs

| English | Norwegian | Other | Meaning | Fits | In lym |
|---|---|---|---|---|---|
| defect, flaw | vevfeil † | sv vävfel; fr défaut; de Webfehler | any fault in the cloth | state | – |
| broken end | rennebrudd † | fr fil cassé; de Kettfadenbruch | a warp thread that snapped | state | – |
| float error, skip | hopp † | fr flotté | a thread that failed to bind where it should | state | – |
| knot, slub | knute | fr bouton; de Noppe | a lump in the yarn | state | – |
| weft bar, streak | – † | fr barre; de Schussstreifen | a visible band from uneven weft | state | – |
| selvedge fault | – † | – | an edge that pulls in or loops | state | Close: `Selvage`, the console pane described above. |
| hole, tear | hull | fr trou; de Loch | a gap in the cloth | state | – |
| snag, pull | – † | fr accroc; de Zugfaden | a pulled loop | state | – |
| darn | stopping, å stoppe | fr reprise; de Stopfstelle | a repair woven across a hole | recipe, artifact | Taken: `darn`, the recipe described above. |
| patch | lapp, lapping | fr pièce; de Flicken | a piece laid over a damaged area | recipe | – |
| mend, invisible mending | usynlig reparasjon † | fr stoppage | a repair that hides the damage | recipe | Close: the fix half of a `burler` round, as above. |
| unravel, run | rakne, å rakne | fr filer; de Laufmasche | threads coming loose from a cut or snag | state | – |
| tension fault | – † | – | warp threads held too loose or tight | state | – |
| misthreading | feiltråing † | fr erreur de rentrage | a thread passed through the wrong heddle | state | – |

## People and trades

| English | Norwegian | Other | Meaning | Fits | In lym |
|---|---|---|---|---|---|
| weaver | vever | sv vävare; fr tisserand; de Weber | works the loom | agent role | Taken: `Weaver`, the board note described above. |
| hand weaver | håndvever | sv handvävare; de Handweber | weaves on a hand loom | agent role | – |
| master weaver | vevmester † | fr maître tisserand; de Webmeister | leads a weaving workshop | agent role | Close: `Master`, the long-lived webster session that reads the plan once and forks one implementer per batch. |
| journeyman | svenn | sv gesäll; da svend; de Geselle | a trained craftsperson who has finished apprenticeship | agent role | – |
| apprentice | lærling | sv lärling; fr apprenti; de Lehrling | a learner under a master | agent role | – |
| guild | laug, skrå | sv skrå; fr corporation; de Zunft | the trade body of a craft | module | – |
| drawboy | – † | – | the helper who lifted warp threads on a drawloom | agent role | – |
| draper | kleshandler † | sv klädeshandlare; fr drapier; de Tuchhändler | sells cloth | agent role | – |
| clothier | – † | fr drapier | organises making and selling of cloth | agent role | – |
| mill hand, operative | fabrikkarbeider † | fr ouvrier | works in a textile mill | agent role | – |
| overseer | oppsynsmann † | fr contremaître | supervises the mill floor | agent role | Close: `tackler`, as above. |
| pattern designer | mønsterkonstruktør † | fr dessinateur; de Musterzeichner | draws the designs and drafts | agent role | Close: `draft`, the unreleased-task board status described above. |
| husflid | husflid | sv hemslöjd; da husflid | home craft, the Norwegian craft movement | module | – |
| teacher of weaving | veveinstruktør † | – | taught rural weaving courses | agent role | – |

## Sources

The Norwegian terms without a mark come from these pages, read for this document:

- Store norske leksikon, article "veving" (snl.no/veving):
  loom parts, structures, processes, people and the organisation names Norges Husflidslag and Den Norske Husflidsforening.
- Det Norske Akademis Ordbok, entry "flatvev" (naob.no/ordbok/flatvev): flatvev, oppstadvev and oppstadgogn.

Neither the Norwegian terms marked `†` nor the Swedish, Danish, French and German columns come from a fetched page.
They are written from general reference knowledge.
They need a check against the references below before a term is adopted:

- Bokmålsordboka and Nynorskordboka (ordbokene.no), for Norwegian spelling and sense.
- Norsk Folkemuseum and DigitaltMuseum records on veving, for tool names such as rennebom.
- Norges Husflidslag publications on veving, for threading, warping and tie-up terms.
- Svenska Akademiens ordlista and Nationalencyklopedin (sv); Den Danske Ordbog (da);
  Trésor de la langue française and Larousse (fr); Duden (de).

The "taken" descriptions come from the package docs under `internal/` and each module's `lym <module> --help`.
They also come from the board's `lym board --help` and `lym board labels` and from the stencil headers in the Bolt checkout.
The board entries `shed-sub-run-block` and the weaver and hardener notes are sources too.
The searches ran against this repository at the commit the document was written on.
Rerun them before relying on a mark.
