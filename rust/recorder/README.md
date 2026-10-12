# recorder

The **recorder** is the device-side component of the Session Recorder system. It
reads audio from a soundcard (ALSA), detects when a signal is present, and streams
the captured audio to the backend.

## What it is

- Captures stereo audio from an ALSA device (48 kHz, 16-bit).
- Runs a level detector (RMS in dB with attack/release smoothing) and only records
  while a signal is present, opening a new session on each silence→signal transition.
- Buffers ready-to-send chunks in a bounded, time-based outbox so a backend outage
  (restart, network blip) doesn't lose audio.
- Discovers backends on the local network via mDNS and streams to every one it finds.
- Optional hardware I/O: status/upload LEDs (Linux sysfs) and a physical "cut session"
  button (`/dev/input/eventNN`).
- Listens for server commands (e.g. remote "cut session") over a streaming RPC.

## Protocol relationship

The recorder **implements a client for the chunksink protocol**.

Protocol names are always written from the **backend's** perspective:

- The backend hosts the **ChunkSink** service — it is the *sink* that receives audio
  chunks. See [`protocols/proto/chunksink.proto`](../../protocols/proto/chunksink.proto).
- The recorder is the *source* of those chunks, i.e. a **ChunkSink client**.

The terms **"chunk source"** and **"chunk sink client"** are equivalent and are used
**only** inside the protocol definitions and the generated stubs (e.g. tonic's
`ChunkSinkClient`). Everywhere else — the binary, the crate, the docs — this component
is called the **recorder**.

## Build

```bash
cd rust/recorder
cargo build --release
```

The protobuf stubs are generated at build time from `../../protocols/proto`
(`chunksink.proto`, `common.proto`) via `build.rs`.

## Run

```bash
./target/release/recorder \
  --recorder-id $(uuidgen) \
  --recorder-name "Living Room" \
  --device default
```

The recorder discovers backends advertising `_session-recorder-chunksink._tcp` over
mDNS and connects automatically. No backend address is configured by hand.

### Options

| Flag | Purpose | Default |
|---|---|---|
| `--recorder-id <uuid>` | Stable unique id of this recorder (required) | — |
| `--recorder-name <name>` | Human-readable name (required) | — |
| `--device <alsa>` | ALSA capture device | `default` |
| `--period-size <frames>` | ALSA period size | `512` |
| `--buffer-size <frames>` | ALSA buffer size | `2048` |
| `--detector-threshold-db <db>` | RMS dB-FS above which a signal is "present" | `-45.0` |
| `--window-time <s>` | RMS analysis window length | `0.25` |
| `--attack-time <s>` | silence → signal transition time | `5.0` |
| `--release-time <s>` | signal → silence transition time | `30.0` |
| `--send-buffer-secs <s>` | seconds of chunks to buffer when no backend is reachable (drops oldest when full) | `120.0` |
| `--led-rec-state <sysfs>` | LED on while recording, blinks on new session | — |
| `--led-upload <sysfs>` | LED pulses on each successful chunk upload | — |
| `--input-event <N>` `--input-keycode <K>` `--input-hold-ms <ms>` | local "cut session" button on `/dev/input/eventN` | — |
| `--fingerprint <true\|false>` | publish a live Chromaprint fingerprint of the last 25 s in the recorder status, every 12 s while recording | `true` |
| `--fingerprint-speed-range <percent>` | also fingerprint each window for assumed playback speeds from -N to +N % (e.g. `8` for a turntable's pitch fader); `0` = off. One after another on one thread, ~0.23 s per speed and window on a Pi 3 | `0` |
| `--fingerprint-speed-step <percent>` | distance between two assumed speeds | `1` |

### Logging

```bash
# info everywhere, silence the chatty mdns_sd crate (the default)
RUST_LOG=info ./target/release/recorder ...

# verbose recorder logs
RUST_LOG=recorder=debug,mdns_sd=warn ./target/release/recorder ...
```

## Architecture

```
 ALSA capture ──► capture ring ──► drain task ──► detector / windowing
 (callback thread)                                      │
                                                        ▼ (while recording)
                                                   send-buffer outbox
                                                        │
                                                        ▼
 mDNS discovery ──► gRPC clients ◄──── chunk sender task (drains when ≥1 backend)
                         ▲
                         └── command listener (GetCommands stream: remote cut)
```

- **Audio callback thread** (RT-pinned) pushes samples into a lock-free ring.
- **Drain task** moves samples off the ring so a slow network send can never stall
  the soundcard.
- **Audio processing task** runs the detector and enqueues finished chunks.
- **Chunk sender task** drains the outbox to every connected backend.
- **Discovery task** keeps the set of connected backends in sync with mDNS.

## Requirements

- Linux with ALSA.
- avahi-daemon running (for mDNS discovery).
- Membership in the `audio` group (or equivalent) to open the capture device —
  only relevant if `--device` names a raw ALSA device (`hw:N`/`plughw:N`)
  directly; not needed when going through PipeWire (see below).

## Audio backend: PipeWire

The recorder's code is unchanged and still talks to a plain ALSA PCM (`src/audio/alsa.rs`)
— there is no PipeWire-specific code. Instead, with `pipewire`, `wireplumber` and
`pipewire-alsa` installed, ALSA's `default` PCM/ctl is transparently redirected
to PipeWire (`/usr/share/alsa/alsa.conf.d/99-pipewire-default.conf`, shipped by
`pipewire-alsa`). That's why the packaged deployment uses `--device default`
rather than a raw `hw:N`/`plughw:N` device: PipeWire multiplexes the hardware
so other local clients on the same machine can use it concurrently, instead of
one process holding it exclusively.

Two things this implies:

- **PipeWire's socket access control is UID-scoped.** The recorder must run as
  the same login user whose session hosts PipeWire/WirePlumber (see the
  `systemd --user` unit in `packaging/`), and any other local process that
  needs to share the interface must run as that same user too — a different
  account is rejected even with matching Unix group membership.
- `--device hw:N`/`plughw:N` bypasses PipeWire and always grabs the interface
  exclusively, PipeWire installed or not — use `--device default` (or another
  PipeWire-backed PCM name) whenever sharing matters.

Verify sharing works with e.g. `arecord -D default -f S16_LE -r 48000 -c 2 -d 3 probe.wav`
run concurrently while the recorder is running — it must succeed instead of
failing with "Device or resource busy".
