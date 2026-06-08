# Architecture: wiki-go-port

## Filstruktur

Modul: `github.com/Knatte18/mhgo`

```
github.com/Knatte18/mhgo/
├── cmd/mhgo/
│   └── main.go          CLI-entrypoint: ruter <module>-argumentet til riktig modul
└── internal/wiki/
    ├── task.go          Task-typen + NewTask / ApplyPatch
    ├── store.go         Store: in-memory CRUD over tasks.json
    ├── layer.go         ComputeLayers, RenderOrder, ExtendedTitle
    ├── render.go        Render: tasks → Home.md, _Sidebar.md, proposal-*.md
    ├── git.go           PathGuard, AtomicWrite, Pull, CommitPush
    ├── lock.go          AcquireWriteLock (gofrs/flock)
    ├── cli.go           RunCLI: parser wiki-subcommands, kaller wiki.go, skriver JSON
    └── wiki.go          Wiki-fasade: writeOp sekvenserer alt
```

## Avhengighetsgraf

```
main.go
  └── wiki.go            (eneste inngangspunkt utenfra)
        ├── lock.go      (skrivelås)
        ├── git.go       (pull / commit / push)
        ├── store.go     (load / mutate / save)
        │     ├── task.go      (Task-type, NewTask, ApplyPatch)
        │     └── layer.go     (ComputeLayers – brukt av ListTasksBrief)
        └── render.go    (produserer wikifiler)
              └── layer.go     (RenderOrder)
```

`internal/wiki` er én Go-pakke — alle filene er i `package wiki` og kan bruke hverandres typer og funksjoner direkte. `cmd/mhgo` er `package main` og er det eneste som importerer `internal/wiki`.

---

## Filene i dybden

### task.go
Definerer `Task`-structen (det som lagres i `tasks.json`) og to hjelpefunksjoner:

- **`NewTask(fields, nextID)`** — bygger en ny Task fra et `map[string]any`. Bruker JSON-roundtrip: felter marshales til JSON og unmarshales inn i Task, slik at feilaktige typer (f.eks. `depends_on` som ikke er `[]string`) gir valideringsfeil.
- **`ApplyPatch(existing, fields)`** — oppdaterer en eksisterende Task. Bruker samme JSON-roundtrip, men starter med å serialisere `existing` til et map, legger nye felter oppå, og unmarshaler til Task. Felter som ikke er i `fields` beholdes uendret.

`Status *string` er en peker fordi `nil` betyr "ikke satt" og utelates i JSON (`omitempty`). En tom streng ville blitt med i JSON.

---

### store.go
Holder task-listen i minnet og eksponerer CRUD-operasjoner.

- **`Store`** — struct med `tasks []Task` og filstien til `tasks.json`.
- **`Load()`** — leser `tasks.json` fra disk. Hvis filen mangler eller er ugyldig JSON, starter den med en tom liste (ingen feil).
- **`Save(wikiPath, relPath)`** — marshaler tasks til formatert JSON og kaller `AtomicWrite` (temp+rename).
- **`validateWrite(snapshot, incoming)`** — kjøres før enhver mutasjon. Sjekker:
  1. Dangling dependency: alle slugs i `DependsOn` må finnes i snapshot
  2. Dep på isolated/deferred task er forbudt
  3. Cycle-deteksjon via DFS (white/gray/black-farging)
  4. Reverse-isolate: ingen eksisterende task kan avhenge av en task som markeres isolated
  5. Reverse-defer: ingen ikke-deferred task kan avhenge av en task som markeres deferred
- **`UpsertTask`** / **`RemoveTask`** / **`SetPhase`** / **`SetDeps`** — enkeltoperasjoner
- **`UpsertTasksBatch`** — validerer alle oppføringer mot en projisert snapshot før noen mutasjon gjennomføres
- **`MergeTasks`** — fjern + upsert + set_phase atomisk: validerer mot snapshot minus fjernede slugs, deretter utfører alt

---

### layer.go
Topologisk sortering av tasks i "buckets".

- **`ComputeLayers(tasks)`** — returnerer `map[slug]bucket`:
  - `"__done__"` — status == "done"
  - `"__deferred__"` — Deferred == true
  - `"Z"` — Isolated == true
  - `"A"`–`"Y"` — basert på dybde i avhengighetsgraf. Dybde 0 = A (ingen avhengigheter), dybde 1 = B, osv. Dybde ≥ 25 er en feil.

  Implementasjon: to DFS-faser. Fase 1 oppdager sykler (white/gray/black). Fase 2 beregner dybde med memoization.

- **`RenderOrder(tasks)`** — returnerer tasks sortert etter bucket-rekkefølge (A→Y, Z, deferred, done), sekundært på ID.
- **`ExtendedTitle(task, layer)`** — returnerer tittel med `[layer]`-suffix for aktive tasks, uten suffix for done/deferred.

---

### render.go
Produserer innholdet i wikiens markdown-filer.

- **`Render(tasks)`** — returnerer `map[relPath]content`:
  - `"Home.md"` — seksjonert per bucket med `# Layer X`/`# Someday`/`# Done`-overskrifter. Hver task: `## **#NNN:** Title [Layer]`, slug-linje, evt. `Depends on:`, evt. brief.
  - `"_Sidebar.md"` — én linje per task, gruppert per bucket med blanklinje mellom grupper.
  - `"proposal-<slug>.md"` — én fil per task med ikke-tomt `Body`.

---

### git.go
Alt som berører filsystem og git.

- **`PathGuard(relPath)`** — avviser tomme stier, absolutte stier (inkl. Windows `C:\...`), og stier med `..`-komponenter.
- **`AtomicWrite(wikiPath, relPath, content)`** — skriver til en temp-fil i samme katalog, deretter `os.Rename` til destinasjon. Rename er atomisk innen samme filsystem — lesere ser aldri en halvskrevet fil.
- **`Pull(wikiPath)`** — `git pull --ff-only`. Returnerer `true` hvis repoet ble oppdatert.
- **`CommitPush(wikiPath, paths, message)`** — `git add → diff --cached → commit → push`. Ved non-fast-forward: prøver `git pull --rebase` én gang, deretter push igjen. Rebase-konflikt → `rebase --abort` + feil.

---

### lock.go
Wrapper rundt `github.com/gofrs/flock`.

- **`AcquireWriteLock(lockPath)`** — oppretter eksklusiv fillås på `tasks.json.lock`. Blokker til låsen er tilgjengelig.
- **`Release()`** — slipper låsen. Låsen frigjøres automatisk hvis prosessen dør (OS-garanti med flock).

---

### wiki.go
Fasaden som brukes av `main.go`. Ingen forretningslogikk her — kun orkestrering.

- **`writeOp(mutate, slugForMsg)`** — kjøres av alle skrive-operasjoner:
  1. Acquirer skrivelås
  2. `Pull` (med mindre `WIKI_SKIP_GIT=1`)
  3. `store.Load()`
  4. Kaller `mutate(store)` — selve endringen
  5. `Render(tasks)`
  6. `AtomicWrite` alle output-filer
  7. Sletter orphan `proposal-*.md` (tasks som mistet body)
  8. `store.Save()`
  9. `CommitPush` (med mindre `WIKI_SKIP_GIT=1`)
  10. Slipper lås (deferred)

- Lese-operasjoner (`GetTask`, `ListTasksBrief`, `ListTasksFull`) bypasser `writeOp` — leser direkte fra disk uten lås.

---

### cmd/mhgo/main.go
Tynn modul-ruter. Tar første argument (`<module>`) og delegerer resten til den modulens egen CLI-handler — `mhgo wiki ...` kaller `wiki.RunCLI`. Hver modul eier sine egne flagg, subcommands og output. Etter hvert som flere moduler legges til utvides kun `switch`-en her.

### internal/wiki/cli.go
Wiki-modulens CLI-handler. `RunCLI(out, args)` parser `[--wiki-path <path>] <subcommand> [json-payload]`, løser wiki-stien (flagg → `MHGO_WIKI_PATH` → `../gowiki`), deserialiserer JSON-argumentet, kaller én metode på `wiki.Wiki`, og skriver resultatet til `out` som JSON. Returnerer exit-koden (0/1) til `main`.

Alle svar er JSON: `{"ok": true, "task": {...}}` ved suksess, `{"ok": false, "error": "..."}` ved feil (exit code 1).

---

## Dataflyt: upsert

Kommando: `mhgo wiki upsert '{"slug": "my-task", "title": "Do something"}'`

```
main.go → wiki.RunCLI (cli.go)
  │  parse args → subcommand="upsert", jsonPayload='{"slug":...}'
  │  json.Unmarshal → fields map[string]any
  │  w.UpsertTask(fields)
  │
wiki.go / UpsertTask
  │  ekstraherer slug for commit-melding
  │  kaller writeOp(mutate, slug)
  │
wiki.go / writeOp
  1. AcquireWriteLock("tasks.json.lock")   ← blokker hvis annen prosess holder låsen
  2. Pull(wikiPath)                         ← git pull --ff-only
  3. store.Load()                           ← les tasks.json fra disk
  4. store.UpsertTask(fields)               ← mutasjon (se under)
  5. Render(store.Tasks())                  ← produser Home.md, _Sidebar.md, proposal-*.md
  6. AtomicWrite(hvert output-filnavn)      ← temp+rename per fil
  7. slett orphan proposal-*.md             ← filer som ikke lenger er i render-outputet
  8. store.Save()                           ← skriv tasks.json atomisk
  9. CommitPush(paths, "wiki: my-task")     ← git add → commit → push (med rebase-retry)
 10. lock.Release()                         ← deferred, frigjøres uansett

store.go / UpsertTask(fields)
  │  slugIndex() → map[slug]*Task
  │  slug finnes ikke → NewTask(fields, nextID())
  │      json.Marshal(fields) → JSON
  │      json.Unmarshal(JSON, &task) → valider typer
  │      sett ID og slug eksplisitt
  │  validateWrite(s.tasks, incoming)
  │      (1) dangling dep-sjekk
  │      (2) dep på isolated/deferred forbudt
  │      (3) DFS cycle-deteksjon
  │      (4) reverse-isolate sjekk
  │      (5) reverse-defer sjekk
  │  append(s.tasks, incoming)
  └→ returnerer Task

cli.go
  outputSuccessWithTask(out, task)
  → stdout: {"ok":true,"task":{"id":0,"slug":"my-task","title":"Do something",...}}
```

---

## Miljøvariabler

| Variabel         | Effekt                                              |
|------------------|-----------------------------------------------------|
| `WIKI_SKIP_GIT=1`| Hopper over pull, commit og push. Kun render+skriv. |
| `WIKI_SKIP_PUSH=1`| Pull og commit kjøres, push hoppes over.           |
| `MHGO_WIKI_PATH` | Setter wiki-katalogen (alternativ til `--wiki-path`)|
