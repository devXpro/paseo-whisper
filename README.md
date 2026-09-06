# paseo-whisper

Local Whisper transcription for [Paseo](https://paseo.sh) dictation. Runs an
OpenAI-compatible endpoint on `127.0.0.1`, backed by
[whisper.cpp](https://github.com/ggml-org/whisper.cpp), so your voice never
leaves the machine.

If you already use **MacWhisper** or **superwhisper**, it reuses the model they
downloaded instead of fetching another copy.

## Why

Paseo ships with local dictation out of the box, using NVIDIA Parakeet. It is
fast and it is genuinely good — but it has limits you will hit if you dictate
technical Russian, German or any other non-English language:

| | Parakeet v3 (built in) | Whisper large-v3-turbo (this) |
|---|---|---|
| Russian WER (FLEURS) | 5.51% | **4.17%** for large-v3; turbo is within noise of it |
| Technical jargon | mangles it: `постгрес` → `прогресс` | noticeably better |
| Vocabulary hints | **impossible** | `prompt` biasing works |
| Mixed-language speech | picks one language per segment | handles switching better |
| Startup cost | none, built in | a service you have to run |
| Memory | loaded on demand | ~1.7 GB resident |

**If Parakeet already works for you, keep it.** This exists for the case where
it does not.

## Requirements

- macOS (Apple Silicon or Intel)
- [Go](https://go.dev) 1.22+ to build
- `whisper-server` from whisper.cpp:
  ```sh
  brew install whisper-cpp
  ```
- A Whisper model — one is found automatically if MacWhisper or superwhisper
  is installed, otherwise it can be downloaded for you.

## Quick start

```sh
git clone <this repo> && cd paseo-whisper
make run
```

On first launch you get a short wizard:

```
  paseo-whisper setup
  ───────────────────
  engine: /opt/homebrew/bin/whisper-server (whisper-cpp 1.7.6)  [homebrew]

  Models found on this machine:
    • large-v3-turbo         superwhisper    1549 MB   [usable]
    • openai_whisper-large-v2 MacWhisper     2946 MB   [not usable]
      CoreML/WhisperKit format, which whisper.cpp cannot load

  How should paseo-whisper get its model?

   1) Reuse large-v3-turbo from superwhisper (1549 MB)  ← recommended
      no download, no extra disk space
   2) Download large-v3-turbo (~1550 MB)
      Best speed/accuracy balance. Recommended for dictation.
   ...
```

Then it asks how to reference a model owned by another app — **hard link** is
recommended: it costs no extra disk space, and the file survives the other
application deleting its copy.

When it is up:

```
  Point Paseo at:  http://127.0.0.1:8099/v1
```

## Non-interactive use

Every choice the wizard offers is also a flag, so this works in scripts and
under `launchd`:

```sh
# Reuse whatever is already on the machine, no questions asked
paseo-whisper serve --yes

# Explicitly reuse superwhisper's model, hard-linked
paseo-whisper serve --model-source superwhisper --link hardlink --yes

# Download a fresh model and stay independent of other apps
paseo-whisper serve --model-source download --yes

# Use your own file
paseo-whisper serve --model ~/models/ggml-large-v3.bin --yes
```

The wizard is skipped automatically when stdin is not a terminal, so a service
never hangs waiting for input.

### Flags

| Flag | Meaning |
|---|---|
| `--model PATH` | use this ggml file |
| `--model-source` | `auto` \| `superwhisper` \| `download` \| `path` |
| `--link` | `hardlink` (default) \| `copy` \| `none` |
| `--port N` | listen port, default `8099` |
| `--threads N` | engine threads; omit to size automatically |
| `--language` | `auto` (default) or an ISO code like `ru` |
| `--prompt-file` | vocabulary hints, see below |
| `--engine PATH` | explicit `whisper-server` path |
| `--save-clips` | keep recordings and transcripts for replay |
| `--yes` | never prompt |
| `--reset` | ignore the saved config and choose again |

Environment variables override the saved config, which is handy for `launchd`:
`PASEO_WHISPER_MODEL`, `PASEO_WHISPER_ENGINE`, `PASEO_WHISPER_LANGUAGE`,
`PASEO_WHISPER_PROMPT`.

### Commands

```sh
paseo-whisper serve      # run the server (default)
paseo-whisper setup      # choose where the model comes from, save, exit
paseo-whisper doctor     # what is installed, what was found, current config
paseo-whisper version
```

`doctor` is the first thing to run when something looks wrong.

## Running it permanently

Dictation breaks silently if the server is not up, so switching Paseo to
whisper installs a launchd agent automatically:

```sh
paseo-whisper paseo use whisper     # also installs the agent, unless --no-autostart
paseo-whisper install               # or install it explicitly
paseo-whisper uninstall
```

The agent starts at login, restarts itself if the process dies
(`KeepAlive` on non-clean exit, throttled to once per 10s), and logs to
`~/.config/paseo-whisper/service.log`.

Installing copies both the binary and the vocabulary out of the working tree:

```
binary: ~/.local/bin/paseo-whisper
terms:  ~/.config/paseo-whisper/terms.txt
plist:  ~/Library/LaunchAgents/com.devxpro.paseo-whisper.plist
```

That matters — an agent pointing into a cloned repository breaks the moment you
move or delete that directory. After installing you can throw the checkout away.

### Why a login agent, and not tied to Paseo

Paseo has no daemon lifecycle hook to attach to: `paseo hooks` records agent
activity, and the config schema has no startup script. Patching the app bundle
would not survive an auto-update.

A login agent sidesteps all of that. Both processes start when you log in, and
the server sits idle at **0% CPU** until audio arrives, so nothing is wasted by
having it always available.

It runs as *your* user, which means it is not running while nobody is logged
in — correct, since Paseo is not running then either.

`doctor` distinguishes the three ways this can be broken:

```
service: not installed          → run: paseo-whisper install
service: installed but not loaded → launchctl rejected it; check service.log
service: installed and loaded   → agent is live
```

## Make targets

```sh
make build      # build into ./bin
make run        # build and start in the foreground
make setup      # choose where the model comes from
make doctor     # engine, CPU, models, service state, config
make install    # run at login via launchd
make uninstall  # stop and remove the agent
make status     # same as doctor
make terms      # mine your chat history for vocabulary
make clips      # list saved recordings
make logs       # tail service and engine logs
make test
```

## Configuring Paseo

You do not edit `~/.paseo/config.json` by hand — this binary does it, backing up
the previous version every time.

### Switching engines

```sh
paseo-whisper paseo status              # which engine is dictation using?
paseo-whisper paseo use whisper         # route dictation to this server
paseo-whisper paseo use parakeet        # back to Paseo's built-in engine
paseo-whisper paseo restart             # restart the daemon and warm agents
```

Or through make:

```sh
make paseo-status
make use-whisper
make use-parakeet
make paseo-restart
```

`paseo status` also probes the endpoint, so a server that is configured but not
running is visible immediately:

```
  Paseo dictation
  ───────────────
  engine:    whisper
  provider:  openai
  model:     large-v3-turbo
  endpoint:  http://127.0.0.1:8099/v1  [reachable]
  config:    /Users/you/.paseo/config.json
```

`paseo use whisper` warns you if nothing is answering on the port yet, rather
than leaving you to discover it when dictation silently fails.

### The restart, and why it is unavoidable

Paseo reads speech settings once at daemon startup. `paseo reload` explicitly
refuses these keys:

```
Warning: These changes require a daemon restart:
  features.dictation.stt.provider
```

`paseo-whisper paseo restart` handles the parts that are easy to get wrong:

1. **Strips `CLAUDE_CONFIG_DIR` and `CLAUDE_PROFILE_NAME` from the environment.**
   A daemon that inherits them pins that config directory for every project
   ([getpaseo/paseo#3618](https://github.com/getpaseo/paseo/issues/3618)), and
   the symptom is empty timelines everywhere except one project.
2. **Waits for the daemon to come back** before doing anything else.
3. **Warms every agent.** After a restart agents load lazily, so timelines
   render empty until something reads them.

Add `--restart` to combine the two steps:

```sh
paseo-whisper paseo use whisper --restart
```

**If you run this from inside a Paseo agent session, the restart ends that
session.** The binary detects `PASEO_AGENT_ID` and warns you first. Chat history
survives; reopen the chat afterwards.

### On mobile

Restarting the daemon drops the relay connection. The phone app reconnects with
a stale cache, so quit and reopen it.

## The model

There is no model to choose: **large-v3-turbo**, always.

That is a decision, not a limitation. Benchmarked on this machine against
`large-v3` using 12 seconds of Russian technical speech:

| | large-v3-turbo | large-v3 |
|---|---|---|
| speed | 1.48s — **8.2x realtime** | 2.61s — 4.6x realtime |
| size | 1.5 GB | 2.9 GB |
| got right | `смёржи`, `компоуз`, `редис`, `графане` | `Закоммить`, `редис`, `графане` |
| got wrong | `Закомить`, `постгресс`, `монго` | `смержи`, `компоус`, `постгрис`, `монго` |

Accuracy was a wash — each model won a word the other lost — while turbo did it
in 57% of the time. Turbo is a distilled large-v3: same encoder, decoder cut
from 32 layers to 4. For dictation you feel a three-second wait on every phrase
far more than one mangled word, so a model menu would only invite worse
decisions.

If you disagree, `--model /path/to/your.bin` still accepts any ggml file. The
tool just will not manage a collection for you.

### Where the file comes from

That part *is* configurable, because it decides whether you download 1.5 GB:

```sh
paseo-whisper serve --model-source auto --yes          # reuse if present, else download
paseo-whisper serve --model-source superwhisper --yes  # insist on superwhisper's copy
paseo-whisper serve --model-source download --yes      # always fetch a fresh one
paseo-whisper serve --model ~/models/ggml-custom.bin   # your own file
```

`auto` prefers a turbo build already on the machine — superwhisper ships one —
and only downloads when nothing usable is found.

### Threads

Left alone, the engine sizes itself: it reads the number of performance cores
and leaves two of them free, clamped to 4-8 threads.

More is not better. Measured on an M4 Pro (10 performance + 4 efficiency cores)
with a 12s Russian clip:

| threads | time |
|---|---|
| 6 | 4.78s |
| **8** | **3.39s** |
| 10 | 4.11s |
| 14 | 7.00s |

The model waits on its slowest worker, so scheduling onto efficiency cores
hurts, and past a point synchronisation costs more than the extra parallelism
buys. `--threads N` overrides the automatic choice if your machine disagrees.

## Vocabulary hints

Whisper accepts an *initial prompt*: text that the decoder treats as what was
said just before. Because the model was trained to continue transcripts
coherently, it carries that text's spelling, casing and script forward. Feed it
`Postgres` and it writes `Postgres` rather than "постгрес".

This is a bias, not a rule. The model may ignore it, and the budget is small:
**224 tokens**, which Cyrillic burns roughly twice as fast as Latin.

### Mine it from your own chat history

A generic word list is worthless — what matters is the jargon *you* use. The
binary reads your Claude Code transcripts and counts what you actually say:

```sh
paseo-whisper terms scan              # preview
paseo-whisper terms scan --write      # write terms.txt
paseo-whisper terms show              # current file and its token cost
```

It reads `~/.claude/projects` plus any per-project `.claude-data/projects`
directories, looks only at your own messages, discards tooling noise and
pasted logs, drops common words in both languages, and reports the token cost
so you know how close to the ceiling you are.

Tune the aggressiveness:

```sh
paseo-whisper terms scan --min-count 5 --limit 80
```

The output is ordered with the **most frequent terms last**, because Whisper
weighs the tail of a prompt more heavily.

What comes out is specific to you. A backend developer working in Russian might
get something like this — note the same word in two spellings, because both get
said out loud, and the transliterations that no generic list would contain:

```
воркспейс, эндпоинт, миграция, хендлер, линтер, рантайм, кеш, конфиг
докер, компоуз, постгрес, редис, кубер, графана, нгинкс, мидлварь
комить, коммить, запушь, заребейзь, смёржи, задеплой, юзать, мемори
```

It also picks up how you actually talk, including the words you would never put
in a curated list. That is the point: the model biases towards what it expects
to hear, so the prompt should match your speech, not your idea of professional
vocabulary.

### Writing one by hand

If you prefer to curate it, three rules from experience:

1. **Only include what actually breaks.** `Docker` and `Redis` are already
   known to the model; listing them burns budget for nothing.
2. **Use the exact forms you speak.** Russian imperatives are what get said out
   loud — `заребейзь`, not the infinitive `заребейзить`. Biasing matches
   strings, so the wrong form misses entirely.
3. **Put the worst offenders last**, where the prompt carries most weight.

Blank lines and `#` comments are stripped. A per-request `prompt` field, if the
client sends one, overrides the file.

The prompt is free in wall-clock terms: measured at 3.39s with it and 3.40s
without, so there is no reason to keep it short beyond the token ceiling.

## How it works

```
Paseo daemon ──POST /v1/audio/transcriptions──▶ paseo-whisper
                                                     │ re-encodes the form
                                                     ▼
                                              whisper-server  (child process,
                                               private port,   model in memory)
```

`paseo-whisper` supervises the engine: it allocates a private port, waits for
the model to load, restarts the child if it dies, and normalises the reply into
the `{"text": ...}` shape the OpenAI client expects. whisper.cpp pads its output
with a leading space and a trailing newline; that is trimmed here.

Everything lives in `~/.config/paseo-whisper/`: `config.json`, `models/`,
`engine.log`, and `service.log` when running under launchd.

## Saved clips

When a transcription comes out wrong, the useful question is whether the model
misheard or whether it never received the whole sentence. Paseo splits audio
into segments with its own VAD before sending them, so a phrase can arrive
already cut in half.

```sh
paseo-whisper serve --save-clips --yes
```

Every recording is kept alongside its transcript in
`~/.config/paseo-whisper/clips/`, most recent 50:

```sh
paseo-whisper clips list        # what was heard, and what came out
paseo-whisper clips play 1      # listen to the last one
paseo-whisper clips path 1      # print the path, to pipe elsewhere
```

Replay a clip against different settings without dictating again:

```sh
curl -F "file=@$(paseo-whisper clips path 1)" \
     -F "prompt=$(grep -v '^#' terms.txt | tr '\n' ' ')" \
     http://127.0.0.1:8099/v1/audio/transcriptions
```

If the audio itself is truncated, the model is not the problem and no prompt
will fix it.

## Troubleshooting

**`whisper-server not found`** — `brew install whisper-cpp`, or pass
`--engine /path/to/whisper-server`.

**MacWhisper models are listed as unusable** — they are, and nothing can be
done about it. MacWhisper stores CoreML/WhisperKit bundles (`.mlmodelc`);
whisper.cpp only reads ggml `.bin` files. superwhisper stores ggml, so it works.
Keep using MacWhisper as you always have — this does not interfere with it.

**Dictation still comes out in English** — Paseo is probably still on Parakeet
v2. Check `features.dictation.stt.provider` and confirm the daemon restarted.

**`engine is still loading the model`** — a large model needs a few seconds on
first start. `curl 127.0.0.1:8099/health` reports `loading` until it is ready.

**Slow transcription — check system load first.** This is the most likely
cause by far, and it has nothing to do with Whisper:

```sh
uptime                    # load average
top -l 2 -n 5 -o cpu      # who is eating the CPU
```

A stuck macOS daemon can hold a core or two for days. On the machine this was
built on, `searchpartyuseragent` and `FindMy` had been burning ~1.5 cores for
four days straight, and dictation took 4.3s per phrase. After
`killall FindMy searchpartyuseragent` the same phrases took **0.7s** — a 6x
difference from one background process, with no configuration change at all.
Those daemons restart themselves cleanly, so killing them is safe.

Only once the machine is actually idle is it worth looking at the engine:
```sh
grep backend ~/.config/paseo-whisper/engine.log
```
`BLAS` means Accelerate on CPU. A newer whisper.cpp may pick Metal instead:
`brew upgrade whisper-cpp`.

**Two apps holding the same model in memory** — they do not. MacWhisper and
superwhisper load their model on demand and release it; only this server keeps
one resident.

## License

MIT
