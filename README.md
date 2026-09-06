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
| `--threads N` | engine threads, default `8` |
| `--language` | `auto` (default) or an ISO code like `ru` |
| `--prompt-file` | vocabulary hints, see below |
| `--engine PATH` | explicit `whisper-server` path |
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

## Make targets

```sh
make build      # build into ./bin
make run        # build and start
make setup      # run the wizard
make doctor     # diagnostics
make install    # install to ~/.local/bin and start at login via launchd
make uninstall  # stop and remove the service
make status     # is it loaded, is it healthy
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

## Vocabulary hints

Whisper accepts an initial prompt that biases spelling. This is how you teach
it project jargon that it would otherwise mangle:

```sh
cp terms.example.txt terms.txt
paseo-whisper serve --prompt-file terms.txt --yes
```

Blank lines and `#` comments are stripped. Keep the result under roughly 200
words — longer prompts start to degrade accuracy rather than help.

A per-request `prompt` field, if the client sends one, overrides the file.

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

**Slow transcription** — check the engine log for the backend it chose:
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
