# atak — Project Specification

## What This Is

A single compiled Go binary that wraps 7-zip and texconv with a Bubble Tea TUI.
It helps S.T.A.L.K.E.R. Anomaly players back up their mod directory and compress textures
to reduce VRAM usage. The target user is non-technical — someone who followed a
YouTube guide to install an Anomaly-based modpack and wants better performance without breaking anything.

**This is not a platform. It is a focused utility with a small, permanent feature set.**

---

## Embedded Binaries

Both compression backends (texconv, optional compressonator-bc7e) and 7-Zip are
embedded into the Go binary via `//go:embed` and build tags, so the correct
platform binary is baked in at compile time. Extracted to a temp directory on
startup, cleaned up on exit.

```
internal/tools/bin/
├── texconv-linux                     # community Linux port of Microsoft's texconv
├── texconv-windows.exe               # official Microsoft build
├── texconv-macos                     # matyalatte macOS universal binary (Intel + Apple Silicon)
├── compressonator-bc7e-linux         # AMD Compressonator fork with bc7e.ispc BC7 encoder (Linux)
├── compressonator-bc7e-windows.exe   # same fork, Windows build
├── compressonator-bc7e-macos         # same fork, macOS universal binary (Intel + Apple Silicon)
├── 7zz                               # 7-Zip standalone Linux binary
├── 7za.exe                           # 7-Zip standalone Windows binary
└── 7zz-macos                         # 7-Zip standalone macOS universal binary (Intel + Apple Silicon)
```

All three platforms ship compressonator-bc7e. `Extract()` still writes the
binary only when the embedded data is non-empty, and callers must check
`EmbeddedTools.CompressonatorPath == ""` to decide availability rather than
special-casing `runtime.GOOS` — that keeps a future platform without a build
from needing changes anywhere but `embed_<platform>.go`.

### Building compressonator-bc7e for macOS

The fork's README §8 lists macOS as out of scope, so the macOS binary is built
from source with `tools/macos/build_flavor.sh` in the fork
(`noisethanks/compressonator`, branch `bc7enc-rdo-integration`). One build per
architecture, joined with `lipo -create` and ad-hoc signed, matching how
texconv-macos and 7zz-macos ship.

**The ISPC host architecture decides whether the encoder is correct.** ISPC
1.19 and later, when the *compiler itself* is an aarch64 build, silently
miscompiles `--` on a varying unsigned int into a no-op
([ispc#3882](https://github.com/ispc/ispc/issues/3882)), which corrupts bc7e's
block bit-packing
([bc7enc_rdo#23](https://github.com/richgel999/bc7enc_rdo/issues/23)). The
damage is not limited to arm64 output — an arm64 ISPC host emits a broken
encoder for the x86_64 target too. Compiled with assertions the failure is
loud (`bc7e.ispc:2890: Assertion failed: *pCur_ofs <= 128`); the release build
passes `--opt=disable-assertions`, so it would instead ship textures that are
quietly ~25 dB PSNR worse.

Two independent routes avoid it, and `build_flavor.sh` accepts either:

1. **Apply [bc7enc_rdo#29](https://github.com/richgel999/bc7enc_rdo/pull/29)**,
   which rewrites the five affected `x--` sites as `x -= 1`. Any ISPC host
   then compiles bc7e correctly. Verified byte-identical to an unpatched
   build made with an x86_64 host, on both targets.
2. **Use the macOS x86_64 ISPC package**, which runs under Rosetta 2 on Apple
   Silicon.

The script refuses only the unsafe combination — an arm64 ISPC against a
bc7e.ispc that still carries the bare decrements. The shipped binary was built
both ways at once: PR #29 applied, x86_64 ISPC 1.31.0 host.

Other fixes carried in the fork, all upstream defects rather than fork changes.
The first four are macOS-specific; the threading one is not:
- The C++ standard probe skipped Apple hosts and left them on C++11, which
  disables the `std::filesystem` path in `cmp_fileio.cpp`. `CMP_GetJustFileExt`
  then returns `dds` instead of `.dds`, `IsDestinationUnCompressed()` compares
  against `".dds"` and answers true for every destination, and the CLI writes
  a **decompressed** DDS while reporting success. Apple now takes the C++17
  branch.
- `CMP_Core_SSE` / `_AVX` / `_AVX512` are x86 intrinsic code compiled with
  `-march=nehalem|haswell|skylake-avx512`; they are skipped on non-x86 targets,
  with the declarations and the BC1 dispatch gated on `CMP_CORE_X86_SIMD`.
- `GetCPUID` was a no-op outside Windows but left its output buffer
  uninitialized, so macOS chose BC1 SIMD kernels from stack garbage. It now
  zero-fills, which also keeps macOS on the same scalar kernels the Linux
  reference build uses.
- The CLI's Apple link list hardcoded `/usr/lib/libz.dylib` and five
  `/usr/local/lib/libIlm*`-era OpenEXR paths. macOS has had no on-disk
  `/usr/lib/libz.dylib` since Big Sur, so the link failed outright.
- **The BC7 worker pool handed slots between threads through a
  `volatile CMP_BOOL run` flag.** `volatile` orders nothing between threads.
  On arm64 the producer could see a slot go idle before the worker's writes
  to the output buffer were visible, then reuse the slot and overwrite the
  input the worker was still reading. Measured on Apple Silicon before the
  fix: stock BC7 produced **10 different outputs from 10 identical runs**,
  the bc7e batched path 3 to 6 distinct outputs from 10, one run lost 19 dB
  of PSNR, and one run segfaulted. The flag is now `std::atomic<bool>` with
  release stores and acquire loads on both sides of the handoff, after which
  every configuration returns a single result across 12 runs and matches the
  `-NumThreads 1` reference exactly. x86-64's store ordering hides this bug
  entirely, which is why the Linux and Windows builds never showed it — it is
  an upstream defect in stock Compressonator, not something the bc7e work
  introduced, and it affects the stock BC7 codec on any weakly ordered CPU.

**Cross-platform output is no longer bit-identical.** The arm64 slice encodes
through bc7e's NEON target and the scalar BC1/BC3/BC4/BC5 kernels compiled for
arm64; both differ in the low bits from the SSE/AVX build. Measured on a mixed
corpus, 18 of 38 format/mip configurations differ byte-wise between the two
macOS slices, while PSNR tracks to within ±0.1 dB. Output is reproducible
within a slice: the same input gives the same bytes on every run.

Against the stock codec on the same machine, bc7e matches on quality and wins
decisively on time — within ±0.7 dB either way across the corpus, and 0.54 s
versus 24.70 s for the same three textures at `-Quality 1.0`. That ordering
(same quality tier, far faster) is what the fork's own README reports, and it
is the reason to choose this backend.

macOS universal binaries contain both x86-64 and ARM64 slices — one binary covers
all Mac hardware. No need to split darwin/amd64 and darwin/arm64 build tags.

Each platform has its own `embed_<platform>.go` with `//go:build` tag and
`//go:embed` directives. All three use the same variable names (`texconvBin`,
`sevenZipBin`, `compressonatorBin`) so the rest of the codebase is
platform-agnostic. See `internal/tools/embed_linux.go` for the canonical pattern.

`EmbeddedTools` fields:
- `TexconvPath` — always populated.
- `SevenZipPath` — always populated.
- `CompressonatorPath` — populated on every platform that embeds a build; empty string when none is embedded.

On startup:
1. Extract every non-empty embedded binary to `os.MkdirTemp`
2. `chmod 0755` (no-op on Windows, harmless)
3. Store paths in an `EmbeddedTools` struct passed through the app
4. `defer tools.Cleanup()` in main

**Binary size:** compressonator-bc7e adds ~9MB on Linux, ~6.4MB on macOS (two
slices in one universal binary) and ~3.5MB on Windows. Current stripped
(`-s -w`) sizes: macOS ~22MB, Linux ~20MB, Windows ~11MB — all under the
historical 25MB target, with macOS now the tightest. Watch this ceiling if
further binaries land.

No other runtime dependencies. The binary must run on any supported platform
without the user installing anything.

---

## Configuration

Config directory is platform-aware via `os.UserConfigDir()` — no hardcoded paths:

```
Linux:   ~/.config/atak/
Windows: %AppData%\atak\
macOS:   ~/Library/Application Support/atak/
```

Each contains:
```
├── config.json     # user prefs (gamma path, backup path, last used settings)
└── profiles.json   # compression profiles — created on first run from embedded default
```

### Profiles Design

**All compression logic lives in `profiles.json` — nothing is hardcoded in the binary.**
This includes format selection, pattern matching, and mip generation. The binary only
knows how to read and apply profiles, not what they should contain.

**First-run behavior:** if `profiles.json` does not exist in the config dir, the tool
copies the embedded default to `~/.config/atak/profiles.json` and shows a
one-time notice screen before proceeding to the main menu:

```
┌─────────────────────────────────────────────────────┐
│  Compression profiles created                       │
│                                                     │
│  A default profiles.json has been created at:       │
│  ~/.config/atak/profiles.json                       │
│                                                     │
│  Edit this file to customize which textures get     │
│  compressed and with which format. Changes take     │
│  effect on the next scan.                           │
│                                                     │
│  Press any key to continue                          │
└─────────────────────────────────────────────────────┘
```

This notice is shown exactly once — never again after the file exists.
Implemented as a dedicated screen `internal/tui/screens/firstrun.go`.
The user owns `profiles.json` from this point forward — the tool never
overwrites it on subsequent launches.

## profiles.json Format Reference

The embedded default and the repo-root `profiles.json` are the canonical examples.
See `profiles.json` at the repo root for current defaults, and
`internal/config/configs/compression_profiles.json` for the embedded seed.

```json
{
  "minFileSizeBytes": 1024,
  "excludePatterns": ["fx_sun*", "fx_*"],
  "profiles": [
    {
      "name": "Profile Name",
      "format": "BC3_UNORM",
      "generateMips": true,
      "patterns": ["*_bump.*", "*/textures/sky/*"],
      "exclude": ["*_bump_detail.*"]
    }
  ]
}
```

### Fields

**Top level:**
- `minFileSizeBytes` — integer, default 1024. Files smaller than this value are
  skipped silently. Protects against compressing stub/placeholder textures which
  produce garbage output (e.g. leopard print artifacts).
- `excludePatterns` — array of glob patterns. Matching rules:
  - Patterns **without** `/` match against the filename (basename) only
  - Patterns **containing** `/` match against the full relative path from the
    mod root, and cover the whole subtree — `*/textures/ui/SquareDOV/*` excludes
    every file under that directory at any depth, regardless of which mod
    provides them. See [Pattern matching rules](#pattern-matching-rules)
  - Matching is case-insensitive on all platforms
  - Evaluated **before** profile matching, so an entry here always beats a
    profile pattern. A pattern listed both here and in a profile makes that
    profile's copy unreachable
  - Files matching any pattern are never compressed regardless of profile match,
    and are reported as `Excluded`

**Per profile:**
- `name` — display name shown in scan results UI
- `format` — BCn compression format. Valid values:
  - `BC1_UNORM` — opaque textures, no alpha. Smallest file size (0.5 bytes/texel).
    Best for: opaque diffuse, environment textures without transparency
  - `BC3_UNORM` — color + alpha (DXT5). Good quality, wide engine support (1 byte/texel).
    Best for: UI, textures with alpha, general purpose safe default
  - `BC4_UNORM` — single channel grayscale. Good for masks, AO maps (0.5 bytes/texel)
  - `BC5_UNORM` — two channel XY normal map data. Required for bump/normal maps (1 byte/texel).
    Do not use BC3 for normal maps — it will produce incorrect lighting
  - `BC7_UNORM` — high quality color + alpha. Best visual quality (1 byte/texel).
    GPU-accelerated on Windows, CPU-only on Linux (~40-60 min for large jobs).
    Best for: high quality diffuse, detailed character/weapon textures
- `generateMips` — mip-chain **policy** for this profile, not an unconditional switch.
  The effective per-file decision is resolved from each source DDS header by
  `ShouldGenerateMips` in `internal/compress/texconv.go`. With the default settings it
  is `profile.generateMips || sourceMipCount > 1`; the `stripMipsWhenDisabled` setting
  (below) changes what `generateMips:false` means:
  - `true` — **always** generate a full chain. Use for world textures (diffuse,
    normal, weapon, terrain, sky) — they are minified with distance and need mips
    even if a careless source shipped without them. Unaffected by `stripMipsWhenDisabled`.
  - `false` — **preserve the source's own choice** (default): keep a full chain when
    the source already had one, generate none when it did not. This is *not* a blanket
    "strip mips" — a flare or scope reticle that ships with mips keeps them; flat UI art
    without mips stays single-level. When the global `stripMipsWhenDisabled` setting is
    on, `false` instead becomes an authoritative "strip": even a mipped source is
    flattened to a single level (smaller output, at the cost of source fidelity).

  This is why one profile can cover a directory whose sources disagree. In a real
  GAMMA install ~1/3 of `anamflares` ship a mip chain (the moon flare has 11 levels
  and renders as a distant billboard that *needs* them) while the rest ship one;
  86% of scope reticles ship mips, 3% of UI icons do. No single static boolean is
  correct for such a folder — the source decides. Stripping a mipped flare or
  reticle to a single level makes it alias and read as a visibly wrong shade when
  minified in-game, since the GPU can no longer pre-average the bright core.
- `patterns` — array of glob patterns matched against filename OR full relative
  path. Path patterns must contain `/`. Order matters — first match wins.
  Examples:
  - `*_bump.*` — matches any file with `_bump` before the extension
  - `*/textures/sky/*` — matches any file under a `textures/sky/` directory
  - `*_d.*` — matches files ending in `_d` before the extension
- `maxTextureSize` — optional integer, default 0 (no limit). When set, caps
  the output texture's maximum dimension while preserving aspect ratio.
  Only applied when at least one dimension exceeds the limit — textures
  smaller than maxTextureSize are never upscaled.

  Implementation: texconv's `-w` and `-h` flags set **exact** pixel dimensions,
  not maximums — passing one without the other would distort non-square textures.
  Both dimensions are computed explicitly before calling texconv, scaling by the
  larger axis so neither exceeds `maxTextureSize`:

  ```go
  if asset.Width >= asset.Height {
      targetW = maxTextureSize
      targetH = int(math.Round(float64(asset.Height) * float64(maxTextureSize) / float64(asset.Width)))
  } else {
      targetH = maxTextureSize
      targetW = int(math.Round(float64(asset.Width) * float64(maxTextureSize) / float64(asset.Height)))
  }
  ```

  Both `-w` and `-h` are passed. Guard against upscaling: only enter this branch
  when at least one dimension exceeds `maxTextureSize` (`||`, not `&&`). Guard
  against division by zero: only enter when `asset.Width > 0 && asset.Height > 0`.

  Recommended use: set on Sky, Terrain, Detail profiles for 4GB VRAM cards.
  Do NOT use on Weapon or Character textures — quality loss is visible up close.
  Do NOT use on cubemap/LOD textures (`*#small*`, `*cube#*`) — texconv handles
  these incorrectly with resize flags, producing files 30x larger than the input.

- `exclude` — optional array of glob patterns. A file matching this profile's
  `patterns` **and** its `exclude` is **declined by this profile**, and matching
  continues with the profiles after it. The file is not dropped.

  This is the mechanism that routes exceptions to a better-suited profile:
  Normal Maps declines `*scope*bump*` and `*lens_bump*`, so scope lens bumps fall
  through to Scope Textures (BC7) rather than being flattened to two-channel BC5.
  It only works because Normal Maps appears *before* Scope Textures — a decline
  with no later match leaves the file `Unmatched`, which is surfaced in scan
  results rather than silently discarded.

  To drop a file outright, use the top-level `excludePatterns`. Those run before
  profile matching and report the file as `Excluded`, so it stays visible in the
  scan; a profile decline is silent by comparison.

### Pattern matching rules
- `*` matches any sequence of characters except `/`
- `?` matches a single character except `/`
- Patterns without `/` are matched against the filename only (basename)
- Patterns containing `/` are matched against the full relative path from the mod root
- **A trailing `/*` is recursive** — it matches everything below that directory at
  any depth, not just direct children. A bare trailing `/` is shorthand for the
  same thing, so `*/textures/ui/` and `*/textures/ui/*` are equivalent
- Patterns are anchored at both ends — a full match, not a substring search
- Matching is case-insensitive on all platforms
- More specific patterns must appear before general ones (first match wins)

The recursive trailing `/*` (equivalently, a bare trailing `/`) is the one
deliberate departure from plain `filepath.Match`, and it is what makes a
path-based exclude usable: `*/textures/ui/SquareDOV/*` has to cover every file
under that directory at any depth, regardless of which mod ships it. Everywhere
else a star stays inside one path segment, so `*/textures/wpn/scope_*` matches
`scope_30mm.dds` but not `scope_reticles/lens.dds`.

All three pattern lists — a profile's `patterns`, a profile's `exclude`, and the
top-level `excludePatterns` — share one implementation (`matchesPattern` in
`internal/scan/walker.go`) so inclusion and exclusion cannot diverge. Previously
inclusion matched by substring while exclusion used `filepath.Match`, which is
how path excludes came to silently miss subtrees.

Patterns are matched against the **mod-root-relative** path, so both walkers
must normalize before matching: `WalkVirtual`'s `relPath` already has that shape,
while `Walk`'s path is relative to `modsDir` and carries a leading mod-name
segment that `modRelPath` strips. Skipping that step leaves an extra segment
that no path pattern can match, silently routing everything to `Unmatched`.

A backslash in a pattern always means a separator and is normalized to `/`,
since patterns are authored rather than observed.

### Compression format quick reference

| Format | Quality | Size | Alpha | GPU accel Linux | Use for |
|--------|---------|------|-------|-----------------|---------|
| BC1 | Good | 0.5 bpt | No | Yes | Opaque diffuse |
| BC3 | Good | 1 bpt | Yes | Yes | UI, general alpha |
| BC4 | Good | 0.5 bpt | No | Yes | Grayscale/masks |
| BC5 | Excellent | 1 bpt | No | Yes | Normal maps only |
| BC7 | Excellent | 1 bpt | Yes | No (CPU only) | High quality diffuse |

bpt = bytes per texel

---

**Profile ordering matters** — profiles are matched in order, first match wins.
More specific path patterns must come before more general ones:
- `*/textures/ui/readables/*` must appear before `*/textures/ui/*`
- `*/textures/sky/night/*` must appear before `*/textures/sky/*`
- Filename suffix patterns (`*_bump.*`) are order-independent since they don't overlap

**The `_bump` suffix is not a reliable indicator of BC5 compatibility.**
Some mods use `_bump` naming for textures that carry more than XY normal data —
scope lens reflection textures in particular often use all 4 RGBA channels for
reflection intensity, gloss, and specular data. Compressing these with BC5 (which
discards B and A channels) causes visual artifacts — loss of reflections, banding
on metallic surfaces.

Mitigation:
- Add `"exclude": ["*scope*bump*", "*lens_bump*"]` to the Normal Maps profile
- Add a dedicated Scope Textures profile using BC7_UNORM before Weapon Textures
- BC7 correctly handles all 4 channels and is appropriate for complex
  metallic/reflective surfaces regardless of naming convention

The two steps are one mechanism, not two independent ones: the `exclude` makes
Normal Maps *decline* those files so they keep matching downward, and the Scope
Textures profile is what catches them. Ordering is load-bearing — Scope Textures
must come after Normal Maps to receive the declines, and the patterns must not
also appear in the top-level `excludePatterns`, which would drop the files before
any profile is consulted.

When in doubt about a texture's channel usage, check with dds_analyze —
`UNCOMPRESSED_RGBA` with a `_bump` suffix means BC5 is wrong for that texture.

**The embedded default** (`internal/config/configs/compression_profiles.json`,
also at repo root as `profiles.json`) ships with broadly correct STALKER conventions:

- **UI/Icons uses BC3** — BC7 was tried here and reverted. UI textures do use
  smooth alpha gradients that BC7 handles better in principle, but the category
  is large (~350 files in a GAMMA install) and BC7 is CPU-only on Linux, so it
  dominated compression time for a difference that is not visible on flat icon
  art with hard alpha edges. Users who want it can set BC7 in their own
  `profiles.json`; there is no runtime cost either way, since BCn decompresses in
  hardware at the same speed regardless of format.
- **Scope Textures use BC7** — scope bump textures often use all 4 RGBA channels
  for reflection/gloss data, not just XY normals. BC5 would destroy B and A.
  This is the only BC7 profile that stays small enough on Linux to be affordable.
- **Weapon Textures, Character/Hands, and Diffuse/Color use BC7** — high visual
  impact textures where quality matters. BC7 is GPU-accelerated on Windows and
  stays manageable on Linux because these categories are smaller than UI.
- The BC7 → BC3 automatic fallback in `texconv.go` remains as a safety net for
  any profile that uses BC7.

**Unmatched files** — DDS files that don't match any profile pattern are surfaced in
scan results as a separate "Unmatched" bucket. They can be skipped or assigned a format
manually in the Scan Results screen before compressing.

**Community sharing** — users can share `profiles.json` files tuned for specific mod
packs. The Settings screen shows the path to `profiles.json` and offers an
"Open in editor" option using `$EDITOR` (Linux) or `notepad.exe` (Windows).

---

## Project Structure

```
atak/
├── main.go
├── go.mod
├── go.sum
├── SPEC.md
├── profiles.json                        # repo-root copy of current default profile
├── bin/
│   ├── texconv-linux / texconv-windows.exe / texconv-macos
│   └── 7zz / 7zz.exe / 7zz-macos
└── internal/
    ├── tools/
    │   ├── embed.go                     # EmbeddedTools struct, extraction, cleanup
    │   ├── embed_linux.go               # //go:embed bin/texconv-linux, bin/7zz
    │   ├── embed_windows.go             # //go:embed bin/texconv-windows.exe, bin/7zz.exe
    │   ├── embed_darwin.go              # //go:embed bin/texconv-macos, bin/7zz-macos, bin/compressonator-bc7e-macos
    │   ├── process_linux.go             # setProcAttr / killProcess — Linux/macOS
    │   ├── process_windows.go           # setProcAttr / killProcess — Windows Job Objects
    │   ├── lockfile.go                  # stale-process lockfile (Linux)
    │   └── lockfile_stub.go             # no-op stubs (Windows/macOS)
    ├── config/
    │   ├── config.go                    # load/save user config and profiles
    │   └── configs/
    │       └── compression_profiles.json  # embedded default profiles seed
    ├── scan/
    │   ├── walker.go                    # walk mod directory or virtual FS, enumerate assets
    │   └── dds.go                       # parse DDS headers, classify format
    ├── modlist/
    │   ├── parser.go                    # parseModList() — reads MO2 modlist.txt
    │   └── virtual.go                   # buildVirtualFS() — assembles virtual filesystem map
    ├── compress/
    │   ├── texconv.go                   # exec.Command wrapper, arg builder, BC7 fallback
    │   └── worker.go                    # goroutine pool, N concurrent jobs
    ├── archive/
    │   └── sevenzip.go                  # backup, restore, list, verify via 7zz
    └── tui/
        ├── model.go                     # top-level AppModel, screen enum, Init/Update/View
        ├── style/
        │   └── style.go                 # lipgloss theme (one place, no scattered styling)
        ├── components/
        │   ├── modpicker.go             # shared fuzzy mod picker (restore + compress)
        │   └── operation.go             # shared progress screen (backup/restore/verify/compress)
        └── screens/
            ├── welcome.go               # path config, first-run detection
            ├── firstrun.go              # one-time profiles.json creation notice
            ├── menu.go                  # main menu hub
            ├── about.go                 # about + third-party licenses screen
            ├── backup.go                # backup manager — all archive ops including restore
            ├── scan.go                  # scanning spinner + live counter
            ├── nav.go                   # navigation helpers
            ├── results.go               # scan results + compression launcher (enter/r/m)
            ├── compress.go              # execution screen using OperationScreen component
            ├── settings.go              # settings editor
            └── summary.go              # completion stats, error list
```

---

## Shared Operation Screen

All long-running operations (backup, restore, verify, compress) use a single
shared `OperationScreen` component at `internal/tui/components/operation.go`.

```
┌─────────────────────────────────────────┐
│  <Operation Title>                      │
│                                         │
│  [spinner]                              │
│  [progress bar]                         │
│  Status: <current file or status line>  │
│  Size: <archive or output size>         │
│  Elapsed: <time>                        │
│                                         │
│  ctrl+c to cancel                       │
└─────────────────────────────────────────┘
```

The component accepts:
- A title string
- A channel of `OperationProgressMsg` (percent int, status string, size int64)
- A cancel function

All four operations feed into this same component — consistent progress feedback
across all operations, and verify gets a progress indicator for free.

---

## Screen Flow

```
Welcome / Path Config
        │
        ▼
   Main Menu ◄──────────────────────────────────────┐
   ├── Scan & Compress                               │
   ├── Backup Manager                                │
   ├── Settings                                      │
   └── About                                         │
        │
        ▼
   Scanning... (async, live counter)
        │
        ▼
   Scan Results
   [enter]  Run Selected Profile
   [r]      Run All
   [m]      Run Selected Mod → ModPicker → Compress
   [q]      Main Menu
        │
        ▼
   Compressing... (OperationScreen)
        │
        ▼
   Summary ──────────────────────────────────────► Main Menu
```

```
Backup Manager
├── list existing backups (size + date)
├── Create New Backup → OperationScreen → done
├── Restore Single Mod → ModPicker → confirm → OperationScreen → done
├── Restore All → confirm → OperationScreen → done
├── Verify Archive → OperationScreen → done
└── Delete Backup → confirm → os.Remove
```

---

## Feature Spec

### 1. Backup Manager

All archive operations live in one screen (`backup.go`). No separate Restore screen.

```
┌─────────────────────────────────────────────────────┐
│  Backup Manager                                     │
│                                                     │
│  Backups in ~/gamma/backup/:                        │
│                                                     │
│  gamma_backup.7z        50.2 GB   Jun 08 14:23      │
│  gamma_backup_old.7z    48.7 GB   May 15 09:41      │
│                                                     │
│  > Create New Backup                                │
│    Restore Single Mod                               │
│    Restore All                                      │
│    Verify Archive                                   │
│    Delete Backup                                    │
└─────────────────────────────────────────────────────┘
```

**Backup list:**
- Scan backup directory with `os.ReadDir` on screen init, filter for `*.7z` files
- Stat each file for size and modification time — no 7z invocation needed
- Display filename, human-readable size, and date above the action menu
- If no backups exist show "No backups found" in that section
- Refresh list after Create New Backup or Delete completes

**Archive selection:**
- When user selects any option except Create New Backup:
  - If only one archive exists — auto-select it, proceed directly
  - If multiple archives exist — show a picker to select which to operate on

**Actions:**
- **Create New Backup** — runs compression via shared `OperationScreen`, no
  archive selection needed
- **Restore Single Mod** — archive picker (if needed) → `ModPicker` component
  → confirm → restore via `OperationScreen`
- **Restore All** — archive picker (if needed) → confirmation dialog with
  warning → full restore via `OperationScreen`
- **Verify Archive** — archive picker (if needed) → verify via `OperationScreen`
- **Delete Backup** — archive picker (if needed) → confirmation → `os.Remove`

- Main menu has four items: Scan & Compress, Backup Manager, Settings, About

- Create a new LZMA solid archive of the full Anomaly mods directory via:
  ```
  7zz a -t7z -m0=lzma2 -mx=<backupLevel> -mfb=64 -md=32m -ms=on -bsp1 <output.7z> <mods_dir> -xr!downloads -xr!Downloads
  ```
  - `-mx=6` — default balanced compression; user-configurable 1-9 in Settings
  - `-mfb=64` — 64 fast bytes, well suited for binary/texture data
  - `-md=32m` — 32MB dictionary, keeps RAM usage sane on large mod lists
  - `-ms=on` — auto solid block sizing, let 7z decide
  - `-xr!downloads`, `-xr!Downloads` — always exclude downloads folder, both cases for Linux case-sensitivity
- Parse 7zz `-bsp1` stderr progress into a Bubble Tea progress bar
- Delete old backups with confirmation
- Verify archive integrity via `7zz t`
- Supports Ctrl+C cancellation — kills 7zz subprocess, deletes partial archive, returns to main menu

**First-run behavior:** if no backup exists and the user navigates to Scan & Compress,
show a warning screen recommending backup first. Do not block — let them proceed if they
explicitly choose to.

### 2. Restore

Two restore modes accessible from the Backup Manager:

**Restore Single Mod:**
- Run `7zz l <archive>` and parse the file listing into a mod name list
- Display as a searchable bubbles/list (fuzzy filter on mod name)
- Confirm dialog showing: mod name, backup date, size on disk
- Restore via:
  ```
  7zz x <archive> -o<parent_of_mods_dir> "mods/<ModName>/*" -r -y
  ```

**Restore All:**
- Confirmation dialog with clear warning: "This will overwrite all mod files
  with backup versions. Continue?"
- Restore via:
  ```
  7zz x <archive> -o<parent_of_mods_dir> -r -y
  ```
- No path filter — extracts everything from the archive

Both modes:
- Stream progress back to UI via shared OperationScreen component
- Support Ctrl+C cancellation — kills 7zz subprocess, returns to main menu

### 3. Scan

- Walk the MO2 mods directory recursively
- For each `.dds` file: read the first 148 bytes, parse the DDS header (including DX10 extended header)
- Skip files where `DDSInfo.Compressed == true` — never re-compress already compressed textures
- Emit `assetFoundMsg` per file (async Cmd) so UI stays live during scan
- Group results by profile for display
- Supports Ctrl+C cancellation — cancels the walk goroutine via context, returns to main menu with message "Scan cancelled"

#### Classification Philosophy

**If it's not explicitly in a profile, don't compress it.**

The tool never blindly compresses unrecognized textures. Texture formats in Anomaly
mods are highly inconsistent across mod authors — engine-specific textures, unusual
formats, and edge cases are common. Auto-compressing unknown textures risks game
crashes and visual corruption.

**Classification is pattern-match only:**
- Files matched by a profile pattern → queued for compression with that profile's format
- Files matched by global `excludePatterns` → always skipped, counted as "Excluded"
- Files not matched by any profile → shown as "Unmatched" (informational only, never compressed)

**No Auto buckets.** The previous Auto (alpha) / Auto (no alpha) header-based
fallback has been removed — it caused engine crashes by compressing engine-specific
textures to unsupported formats.

**Result buckets in scan results:**
- One bucket per named profile (from profiles.json) — compressible, selectable
- `Unmatched` — files with no profile match, shown with count but greyed out and
  not selectable for compression. Label: "Add patterns to profiles.json to compress these."
- `Excluded` — files matching global excludePatterns, shown for transparency
- All buckets shown regardless of count (zero-hit profiles still render)

#### Profile-Level Exclusions

Profiles support an optional `exclude` array — patterns that match the profile's
`patterns` but which this profile should **decline**. A declined file is not
dropped; matching continues with the profiles below it, and only a file that
reaches the end with no match becomes `Unmatched`.

```json
{
  "name": "Normal Maps",
  "format": "BC5_UNORM",
  "generateMips": true,
  "patterns": ["*_bump.*", "*_normal.*"],
  "exclude": ["*scope*bump*", "*lens_bump*"]
}
```

Here Normal Maps claims bump maps generally but hands scope and lens bumps to
whichever later profile wants them — Scope Textures, at BC7. Declining is
therefore a routing decision, not a skip. Use the top-level `excludePatterns`
when the intent is genuinely "never compress this file".

#### Global Exclusion Patterns

`profiles.json` supports a top-level `excludePatterns` array. Patterns without
`/` match against the filename (basename). Patterns containing `/` match against
the full relative path from the mod root — useful for excluding specific texture
directories regardless of which mod provides them:

```json
{
  "excludePatterns": [
    "fx_sun*",
    "*_lm.*",
    "*/textures/ui/SquareDOV/*"
  ],
  "profiles": [...]
}
```

Evaluated before any profile matching. If a file matches `excludePatterns`, it is
skipped and counted as "Excluded".

**Counters on scan results screen:**
- `___ to compress` — total files matched by profiles (excluding excluded files)
- `___ skipped (compressed)` — files already compressed, skipped by scanner
- `___ unmatched` — uncompressed files with no profile match (informational)
- `___ excluded` — files matching global excludePatterns

**Unknown format handling:**
Files where the FourCC or DXGI format code is not recognized are treated as
unmatched — shown in the Unmatched bucket, never compressed.

#### Scanner Exclusions

Directory exclusions are user-configurable via `scanExclusions` in `config.json`.
They share the profiles.json pattern syntax: a pattern **without** a separator matches a
directory (or mod) **name** anywhere, and a **path** pattern matches a directory's
mod-root-relative path, so an exclusion can target a nested subtree and not only a
top-level folder. The matched directory and everything beneath it is skipped — in the
directory walk via `filepath.SkipDir`, and in mod-output mode by dropping the whole mod
(name patterns) or every file under the directory (path patterns), so the two scan modes
agree. Name matching is case-insensitive, as elsewhere in the pattern system. A path
pattern names its directory in any of three equivalent forms: `dir`, `dir/`, `dir/*`.

Default value shipped in config:
```json
"scanExclusions": [".*", "downloads", "Downloads", "G.A.M.M.A. UI"]
```

- `.*` — skips all hidden directories (e.g. `.Grok's Modpack Installer`, `.git`)
- `downloads` / `Downloads` — skips the Anomaly/GAMMA downloads folder (both entries are
  now redundant since matching is case-insensitive, but kept for clarity)
- `G.A.M.M.A. UI` — skips the GAMMA UI mod directory. Compressing main menu assets
  causes excessive loading times — confirmed by community testing

Note: SquareDOV minimap textures are excluded via `*/textures/ui/SquareDOV/*` in
`excludePatterns` in `profiles.json` — path-based exclusion covers all mods that
ship those textures regardless of mod name or number prefix. The same directory could
instead be pruned entirely with `*/textures/ui/SquareDOV` in `scanExclusions`; the
difference is that a profile exclude reports the files as Excluded, while a scan
exclusion skips them silently before classification.

Surfaced in the Settings screen as an editable list — users can add or remove patterns.

**Hardcoded exclusions (never user-configurable):**
- Files where `DDSInfo.Compressed == true` — never re-compress already compressed textures
- Files without `.dds` extension — only DDS files are processed

**Configurable exclusions (in profiles.json):**
- `minFileSizeBytes` — top-level field in profiles.json, default 1024. Files smaller
  than this value are skipped. Stub/placeholder textures are typically under 200 bytes;
  the smallest real usable texture (16x16 uncompressed RGBA) is ~1KB. Counted in the
  skipped total, not surfaced as errors. Users can lower this if they have legitimate
  tiny textures, or raise it to skip small textures entirely.

### 4. Compress

#### Compression — No Config Screen

There is no separate compression config screen. The scan results screen is the
compression launcher. All compression is initiated directly from scan results
via keybindings:

```
Scan Results keybindings:
  [enter]   run selected profile (whichever profile row is highlighted)
  [r]       run all profiles
  [m]       run selected mod — opens ModPicker, then compresses that mod only
  [q]       back to main menu
```

**Run Selected Mod** is the recommended first-time workflow — surface this in
the scan results screen as a hint: "Press [m] to compress a single mod first".

The mod picker for [m] uses the shared `ModPicker` component. On selection,
assets are filtered to the chosen mod before passing to the worker pool.

#### Compression Execution

- Worker pool: `workerCount` concurrent backend processes (default 1, configurable in Settings)

- **Backend abstraction.** Two backends implement `compress.Backend`
  (`Name() string`, `Compress(ctx, job) CompressionResult`):
  - `TexconvBackend` — wraps the existing texconv path; behavior below is
    unchanged from the pre-abstraction implementation.
  - `CompressonatorBackend` — invokes the embedded compressonator-bc7e CLI
    (`internal/compress/compressonator.go`). AMD Compressonator fork with the
    CPU-side BC7 codec replaced by `bc7e.ispc` from richgel999/bc7enc_rdo;
    GPU codec paths compiled out of the fork. Available on all three
    platforms.

  `worker.RunPool` selects the primary backend once per run from
  `config.CompressionBackend` and passes it plus an optional fallback into each
  worker goroutine. No per-file backend switching except the explicit
  `maxTextureSize` fallback below.

- **compressonator-bc7e is always CPU, on every platform.** The fork ships with
  its GPU codec paths compiled out — the GPU path isn't guaranteed to work and
  is never attempted. `compressonatorArgs` **hardcodes `-EncodeWith CPU` on
  every invocation** rather than relying on the binary's default, so a future
  upstream change to the default can't quietly re-enable a broken GPU path.
  This is not user-configurable. It also means Windows users choosing this
  backend give up texconv's DirectX BC7 acceleration on purpose — the tradeoff
  buys the fork's fixed BC7 p-bit correctness on every platform. Output is
  bit-identical between the x86-64 builds; the macOS arm64 slice matches on
  quality but not byte-for-byte (see Embedded Binaries). The active backend name and its CPU/GPU character are surfaced
  in the compress `OperationScreen` title (e.g.
  `Backend: compressonator-bc7e (CPU)` vs. `Backend: texconv (GPU for BC7)`)
  so mid-run timing expectations are legible.

- **maxTextureSize fallback (compressonator → texconv).** compressonator-bc7e's
  CLI exposes mip controls but no exact-size resize flag (no `-w`/`-h`
  equivalent). Rather than silently ignore `maxTextureSize`, `dispatch()` in
  `internal/compress/backend.go` routes any file that needs resizing through a
  texconv fallback backend for that file only. Mirrors the existing texconv
  BC7 → BC3 fallback philosophy — automatic, transparent, recorded. The
  backend that actually processed each file is captured in the new
  `CompressionResult.Backend` field so the summary and error UI can attribute
  mismatches correctly.

- **DDS-reader gap fallback (compressonator → texconv, narrow match).**
  compressonator-bc7e's DDS loader rejects some subvariants DirectXTex handles
  — most commonly DX10-header DDS with an sRGB DXGI_FORMAT (e.g.
  `DXGI_FORMAT_B8G8R8A8_UNORM_SRGB` = 91). When primary is compressonator and
  its stderr contains the substring `Could not load source file`, `dispatch()`
  retries the file through the texconv fallback. Match is deliberately narrow
  — a blanket "any compressonator failure retries" would silently absorb
  unrelated future failure classes (argv bugs, missing binary, permissions,
  format mismatch) into a texconv retry that hides real bugs. On success the
  fallback's `Backend` value (`"texconv"`) is preserved so mismatches surface
  in the summary/error UI. Regression coverage:
  `internal/compress/dispatch_srgb_test.go`.

  On **double failure** (fallback also rejects the file, e.g. genuinely
  corrupt DDS): the file is surfaced as a normal per-file failure — same
  error list, same counting, same UI — with no special "fallback also failed"
  state. `CompressionResult.Stderr` is the concatenation
  `compressonator-bc7e:\n<primary stderr>\n---\ntexconv:\n<fallback stderr>`
  so the failure record carries full debug context from both backends.
  `CompressionResult.Backend` is blanked (`""`) in this case because neither
  backend produced output — misattributing to either would be misleading.

- **argv-dump-on-failure diagnostics.** When compressonator exits non-zero,
  `Compress` prepends `argv: <bin> <args...>` to the diag string carried in
  `CompressionResult.Stderr`. Keeps failure records self-contained — no need
  to re-run under a debugger to see the child's command line.

- Per-file texconv invocation:
  ```
  texconv -f <FORMAT> -m 0|1 -if CUBIC -gpu 0 -y -nologo [-w <W> -h <H>] -o <output_dir> -- <input_file>
  ```
  Note: `--` separator is required before input path — paths starting with `/`
  are interpreted as flags without it.

  `-m 0` full mip chain / `-m 1` top level only. The choice is resolved **per file**,
  not per profile, via `ShouldGenerateMips` in `internal/compress/texconv.go`:
  a profile's `generateMips:true` always yields `-m 0`, while `generateMips:false`
  yields `-m 0` only when the source DDS already had a chain (`mipMapCount > 1`)
  and `-m 1` otherwise. So a mipped source is never flattened and a mipless world
  texture still gets a chain forced by its profile. The `stripMipsWhenDisabled`
  setting overrides the `generateMips:false` branch to always yield `-m 1`
  (authoritative strip); `generateMips:true` is unaffected.

  `-if CUBIC` cubic interpolation for mip generation (better quality)
  `-gpu 0` GPU accelerated compression (DirectX GPU on Windows, CPU fallback on Linux)
  `-nologo` suppress Microsoft header output
  `-w <W> -h <H>` only added when `maxTextureSize > 0` — both dimensions computed
  explicitly to preserve aspect ratio (see `maxTextureSize` field above)

  Note: `-bc x` (quick BC7 encoder) intentionally removed. The exhaustive BC7
  encoder produces significantly better quality on metallic and reflective surfaces
  (scopes, weapons). On Windows with GPU acceleration there is no meaningful speed
  impact. Linux users running BC7 profiles should expect longer compression times.

- **BC7 → BC3 automatic fallback:** If texconv exits non-zero with BC7_UNORM,
  automatically retry with BC3_UNORM. Matches proven bash script behavior.
  `CompressionResult` records the actual format used after fallback. This
  fallback is texconv-specific — compressonator-bc7e has no analogous BC7
  fragility (bc7e.ispc is the whole point of the fork).

- Per-file compressonator-bc7e invocation:
  ```
  compressonatorcli -fd <BC1|BC3|BC4|BC5|BC7> -EncodeWith CPU -Quality 1.0
                    -noprogress (-mipsize 1 | -nomipmap)
                    <input> <output.dds>
  ```
  Positional `output.dds` avoids texconv's extension-case rename dance
  entirely. `-mipsize 1` produces a full mip chain to a 1-pixel minimum
  (equivalent to texconv's `-m 0`); `-nomipmap` is the mipless branch. Mip
  decision reuses `ShouldGenerateMips` — no per-backend re-implementation.
  Unsupported format strings fail loudly (bug in upstream code, not a
  silent default). Compressonator writes progress/diagnostics to stdout, not
  stderr — the backend carries both into `CompressionResult.Stderr` so the
  failure UI shows the actual error rather than empty.

- **Extension case preservation:** texconv lowercases the output extension by
  default — `texture.DDS` becomes `texture.dds`. On Linux (case-sensitive
  filesystem) this creates a second file, leaving the original uncompressed
  `.DDS` file untouched. Fix: after successful texconv run, if the output path
  differs from the original asset path, rename the output to match the original
  filename exactly via `os.Rename`. Only applies in-place mode — in mod output
  mode renaming to `asset.Path` would overwrite the source file.

- Capture stderr per file into `CompressionResult`
- Emit progress per file via `OperationProgressMsg{Percent, Status, Size, Done}`
- Per-file errors accumulate separately and are shown on the summary screen —
  individual file failures do not abort the job
- Uses shared `internal/tui/components/operation.go` for progress display —
  same spinner/size/elapsed UI as backup and restore
- On completion: transition to summary screen with success count, error count,
  estimated VRAM delta
- Error list is navigable; failed files are shown with their stderr output
- No retry with different settings — if a file failed, fix profiles.json and rescan

- **Fallback surfacing on summary screen.** All three fallbacks (BC7→BC3,
  compressonator→texconv resize, compressonator→texconv DDS reader gap) set
  `CompressionResult.FallbackReason` to one of the `compress.Fallback*`
  constants on success. The Compress→Summary bridge accumulates a
  `FallbackCounts` map (reason → count) and a `Fallbacks` string slice
  (per-file drill-down lines). The summary screen renders:
    - **Per-reason counts** — one warning-colored line per reason with a
      nonzero count (e.g. `↷  1 compressonator-bc7e → texconv (DDS reader
      gap)`). Clean runs show no fallback lines — no "0 fallbacks" noise.
    - **`Backend fallbacks` navigable section** — separate from Errors, using
      the same j/k window-of-10 cursor mechanism. `tab` toggles cursor focus
      between the two sections; a `▸` marker on the section header shows
      which is focused. Fallbacks render in warning color (not danger) —
      they succeeded, they just weren't handled by the primary backend.
    - No live per-file backend indicator during the run — the CPU/GPU hint on
      the run-header title covers the run-wide backend choice; per-file
      fallbacks only surface post-run to avoid mid-run noise.

  Rationale: `CompressionResult.Backend` and `FallbackReason` were set on
  every path but not read anywhere in the UI before this — silent fallbacks
  made the sRGB reader-gap failure look like a clean compressonator success
  in a real 7GB run. The summary surface closes that gap without adding
  mid-run noise.

#### Cancellation

All long-running operations (backup, restore, compress) must support Ctrl+C cancellation:

- A `context.WithCancel` context is created at operation start and stored in the
  top-level model
- The cancel function is called when Ctrl+C is pressed during an active operation
- Workers receive the context and check `ctx.Done()` between files
- Subprocess kill on cancellation — set `Setpgid: true` on start, then kill the
  entire process group on cancel so any children 7zz or texconv may have spawned
  are also killed. `cmd.Wait()` after kill returns an error — swallow it as expected
- Partial output files are deleted on cancel
- After cancellation, the app returns to the main menu with message: "Operation cancelled"
- Ctrl+C on the main menu or any non-operational screen exits the app normally
- Implemented purely through Bubble Tea key messages — do NOT use `os/signal`

#### Crash / Orphan Process Mitigation

Platform-specific process management in `internal/tools/process_<platform>.go`.
All platforms expose the same interface: `SetProcAttr(cmd)`, `KillProcess(cmd)`,
`NewJob()` / `AssignJob()` / `CloseJob()`.

**Linux/macOS** (`process_linux.go`, `//go:build linux || darwin`):
- `SetProcAttr` sets `Setpgid: true` — subprocess gets its own process group
- `KillProcess` sends `SIGKILL` to the entire process group (`-pid`)
- Crash mitigation via lockfile (`lockfile.go`): write PID on start, delete on
  clean exit, kill stale PID on next startup. `lockfile_stub.go` no-ops this on Windows.

**Windows** (`process_windows.go`):
- `SetProcAttr` creates a Windows Job Object with `KILL_ON_JOB_CLOSE` — when the
  Go process exits (clean or crash), Windows automatically kills all job members
- `KillProcess` calls `cmd.Process.Kill()` for the explicit cancel case
- No lockfile needed — Job Objects provide crash cleanup automatically

### 5. Settings

- Anomaly mods directory path — auto-detect from common locations:
  - Linux: `~/Games/Anomaly/mods`, `~/Anomaly/mods`, `$MO2_GAME_PATH`
  - Windows: `C:\Games\GAMMA\mods`, `D:\GAMMA\mods`, `%MO2_GAME_PATH%`
  - All path handling via `filepath.Join` — no hardcoded separators anywhere
- Backup archive path
- Backup compression level: integer 1-9, default 6
  - Displayed in Settings as a text input with inline guide:
    ```
    Backup Compression Level (1-9): [6]

    1-3  Fast compression, larger archives
    4-6  Balanced — recommended for most systems
    7-9  Maximum compression, significantly slower
    ```
  - Validated on input — reject values outside 1-9, non-numeric input reverts to previous value
  - All other 7z flags (`-mfb=64 -md=32m -ms=on -xr!downloads -xr!Downloads`) are hardcoded, not user-exposed
- **Compression workers** (`workerCount`) — concurrent texconv processes.
  Default: 1. Each worker pegs one CPU core. This setting applies to texconv only, not 7-Zip.
  Settings screen label: "Compression workers (texconv)"
- Compression is always in-place — no staging directory option
  - The backup system is the safety net; restore from backup if needed
  - Removes user confusion and config complexity
- Scan exclusions — editable list of glob patterns, default: `[".*", "downloads", "Downloads", "G.A.M.M.A. UI"]`
- **Strip mips when disabled** (`stripMipsWhenDisabled`) — bool toggle, default `false`.
  When off, a profile's `generateMips:false` preserves a mipped source's chain (the
  source decides). When on, `generateMips:false` becomes authoritative and strips the
  chain to a single level regardless of source — smaller output at the cost of fidelity
  for flares/reticles. Never affects `generateMips:true`. Settings screen label:
  "Strip Mips When Disabled". Resolved in `compress.ShouldGenerateMips`.
- **Compression backend** (`compressionBackend`) — string enum, valid values
  `"texconv"` (default) and `"compressonator-bc7e"` (CPU-only). Both are
  available on all three platforms. Unknown or empty values are coerced to
  `"texconv"` on load, so a hand-edited or future-dated config can never name a
  backend this build has no implementation for. Toggle in Settings
  with `space` / `←` / `→` — two-way selector, not free text.
- Persist to `os.UserConfigDir()/atak/config.json`

Full config.json schema (see `internal/config/config.go` for canonical struct):
```json
{
  "modsDir": "/home/user/Anomaly/mods",
  "backupDir": "/home/user/Anomaly/backup",
  "workerCount": 1,
  "backupLevel": 6,
  "scanExclusions": [".*", "downloads", "Downloads", "G.A.M.M.A. UI"],
  "modOutputMode": false,
  "modOutputName": "ATAK",
  "modlistPath": "",
  "stripMipsWhenDisabled": false,
  "compressionBackend": "texconv"
}
```

Default compression is in-place. The backup system is the safety net.
When `modOutputMode` is true and `modlistPath` is set, ATAK uses the
virtual filesystem approach — see Mod Output Mode section.

---

## Bubble Tea Conventions

These must be followed consistently or the architecture drifts:

- `Update` is pure. No I/O, no side effects, no blocking calls.
- All I/O happens in `Cmd` functions that return a `Msg`.
- Sub-screens each have their own `Model`, `Update`, and `View`.
- Top-level `AppModel` delegates to the active screen's Update/View.
- All styling is in `tui/style/style.go` via lipgloss. No inline color strings elsewhere.
- Screen transitions happen by returning a new screen enum value from Update.
  The top-level model swaps the active screen on the next render cycle.

## Persistent Scan State

Scan results must persist in `AppModel`, not in `ResultsModel`. This prevents
state loss when navigating away from and back to the results screen.

`ResultsModel` is a view over the data, not the owner of it. When navigating
back to results, reconstruct `ResultsModel` from `AppModel.scanAssets` and
`AppModel.scanSkipped` — never lose scan data on screen transition.

---

## About / Licenses Screen

Accessible from the main menu (`internal/tui/screens/about.go`). Displays version,
project URL, and a scrollable section with all third-party licenses:
1. texconv (Texconv-Custom-DLL) — MIT
2. 7-Zip — LGPL v2.1
3. Charmbracelet UI dependencies (bubbletea, bubbles, lipgloss) — MIT
4. **compressonator-bc7e** (optional backend, all platforms) — **dual-licensed:
   AMD Compressonator MIT + `bc7e.ispc` Apache License 2.0**. The Apache 2.0
   grant requires the release to identify the incorporated Apache-2.0
   component, and the About screen carries that attribution verbatim
   alongside the license text:
   > This software incorporates bc7e.ispc from richgel999/bc7enc_rdo,
   > © Richard Geldreich / Binomial LLC, licensed under the Apache License,
   > Version 2.0.

   **Do not drop the Apache-2.0 side thinking the MIT entry covers the whole
   backend** — they are two separate license grants on distinct code paths.
   Full text for both licenses ships in `licenses/compressonator-bc7e/` and is
   also inlined in `about.go`. `nlohmann/json` (MIT) is linked into
   Compressonator and its notice ships alongside. ETCPack and its Ericsson
   SLA are **not** part of this build's license surface (per the fork's own
   README §3) and must not appear here even if a future contributor sees the
   name in Compressonator sources.

This satisfies matyalatte's redistribution requirement — license notice is
present in the distributed binary's about screen — and the Apache 2.0
attribution requirement for bc7e.ispc.

Version string injected at build time via `-ldflags "-X main.version=v0.1.0"`.
In development builds without the flag, version displays as `dev`.

---

## What This Is Not

To keep maintenance footprint small, the following are explicitly out of scope:

- 32-bit builds (x86-64 and ARM64 only)
- Plugin or extension system
- Network features (no auto-update, no telemetry, no download)
- Support for archive formats other than 7z
- Texture formats other than DDS input / BCn output
- MO2 integration beyond reading the mods directory path

---

## Mod Output Mode (v0.2.0)

An optional non-destructive compression mode that outputs compressed textures
to a single flat mod folder compatible with MO2, rather than compressing in-place.

### Overview

When a MO2 `modlist.txt` is provided, ATAK builds a virtual filesystem representing
the final merged modlist — the same view MO2 presents to the game. Only the winning
file for each texture path is compressed. Output goes to a single flat mod folder
(`ATAK/` by default) that the user adds as the highest-priority mod in MO2.

```
Before:                          After (MO2 load order):
mods/                            mods/
├── 001- Mod A/                  ├── 001- Mod A/          ← originals untouched
│   └── gamedata/tex/ak74.dds   ├── 002- Mod B/          ← originals untouched
├── 002- Mod B/                  └── ATAK/               ← add as highest priority
│   └── gamedata/tex/ak74.dds       └── gamedata/
└── ATAK/  ← new                         └── tex/
    └── gamedata/                             └── ak74.dds ← compressed winner
        └── tex/
            └── ak74.dds  (002- Mod B wins)
```

### Configuration

New fields in `config.json`:
- `modOutputMode` — enable/disable. Default false (in-place mode)
- `modOutputName` — name of the output mod folder. Default "ATAK".
  Created as `<modsDir>/<modOutputName>/`
- `modlistPath` — path to MO2 `modlist.txt`. Optional — if not set,
  Mod Output Mode falls back to in-place behavior

### modlist.txt parsing

MO2's modlist.txt format:
```
+High Priority Mod
+Medium Priority Mod
-Disabled Mod
+Low Priority Mod
```

- `+` prefix = enabled, `-` prefix = disabled
- Order = priority (FIRST line = highest priority in MO2)

Parsing: read file, filter to `+` lines, strip `+` prefix, do NOT reverse —
modlist.txt already lists high priority first. Return `[]string` of enabled mod
names in priority order (high → low). See `internal/modlist/parser.go`.

### Virtual filesystem

`buildVirtualFS` in `internal/modlist/virtual.go` builds a flat
`map[relPath]absoluteSourcePath` by iterating the mod list from low to high
priority, so higher-priority mods overwrite lower-priority entries for the same
relative path. Result: every key maps to the winning (highest-priority) source
file. Passed to the scanner instead of walking the mods directory directly.

### Output structure

Compressed files are written to `<modsDir>/<modOutputName>/gamedata/...`:

```
mods/ATAK/
└── gamedata/
    └── textures/
        └── wpn/
            └── ak74_d.dds  ← compressed version of whichever mod won
```

Standard flat MO2 mod structure. User adds `modsDir/ATAK/` as a mod in MO2
and places it at the top of the load order.

### Scanner behavior in Mod Output Mode

- The output folder (`ATAK/` or custom name) is automatically added to
  `scanExclusions` — never scanned, never compressed
- Scan operates on the virtual filesystem, not the raw mods directory
- File counts reflect unique files (duplicates across mods deduplicated)
- Already-compressed files in the virtual filesystem are skipped as normal

### Incremental runs

On subsequent runs, ATAK checks if the output file already exists in the
output folder before compressing. If it exists, the file is skipped.

This allows incremental updates — add new mods, rerun, only new files
are compressed. Existing compressed files in the output folder are untouched.

**User communication — incremental skips must be surfaced clearly:**

The scan results and execution screens show a separate counter:
`X already in output folder` — distinct from `skipped (compressed)` which
refers to already-compressed source files.

If ALL files are skipped because the output folder already has everything:
- Summary screen shows: "Nothing to compress — all files already exist
  in output folder. Delete <outputDir> to force recompression."
- This is displayed prominently, not buried

Settings screen shows the output folder path with a hint:
```
Output folder: mods/ATAK/   [Delete to recompress all]
```

**To force full recompression:** delete the output folder and rerun.
ATAK does not provide a built-in "force recompress" button — deleting
the folder is the explicit user action.

### Settings screen

New fields in Settings:
- **Mod Output Mode** toggle (on/off)
- **Output mod name** text field (default "ATAK"), shown when toggle is on
- **MO2 modlist.txt path** text field, shown when toggle is on
  - Shows warning if file not found

### Fallback behavior

If `modlistPath` is empty or the file cannot be read:
- Log a warning
- Fall back to scanning the raw mods directory (current behavior)
- Mod Output Mode still writes to the output folder, but may include
  duplicate files (one per mod that has them)

---

## Community Profiles

`profiles.json` at the repo root is the current recommended default — it uses BC7 for
Weapon Textures, Character/Hands, and Diffuse/Color, and is also the embedded seed
that ships in the binary (`internal/config/configs/compression_profiles.json`).

Users drop alternative `profiles.json` files into `~/.config/atak/profiles.json` to
switch configurations. Community members can contribute profiles for specific mod packs
as PRs — low barrier to contribution, high value for the ecosystem.

Recommended for 4GB VRAM cards: set `"maxTextureSize": 1024` on Sky, Terrain, and
Detail profiles. A `lowvram.json` preset in the repo would be a natural contribution.

---

## Future / Post-1.0

- **Atomic compression** — compress to staging directory, verify all files
  succeeded, then diff-apply in one pass. Failed jobs leave the mod directory
  untouched. Planned for v1.1.
- **Scan metadata persistence** — store scan results and compression history
  to disk. Enables: "already done" tracking, incremental rescans. Requires a
  simple local database or JSON state file.
- **stalker-update** — separate binary, same visual identity, handles Anomaly modpack updates selectively. Dependent on community reception of atak.

---

## Dependencies

```
github.com/charmbracelet/bubbletea   # TUI runtime
github.com/charmbracelet/bubbles     # list, textinput, progress, spinner
github.com/charmbracelet/lipgloss    # styling
```

No other external dependencies. Standard library only for everything else.

---

## Build

```bash
# Development
go run ./main.go

# Release — Linux x86-64, static
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
  -ldflags="-s -w -X main.version=v0.1.0" \
  -o atak-linux ./main.go

# Release — Windows x86-64
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build \
  -ldflags="-s -w -X main.version=v0.1.0" \
  -o atak-windows.exe ./main.go

# Release — macOS (universal embedded tools, Go binary is amd64)
CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build \
  -ldflags="-s -w -X main.version=v0.1.0" \
  -o atak-macos ./main.go

# goreleaser handles all targets in CI — version injected from git tag
```

Version is injected at build time via `-X main.version=<tag>`. In development
builds without the flag, version displays as `dev`.

## Build Targets

- `linux/amd64` — primary, tested by maintainer
- `windows/amd64` — supported, community-tested
- `darwin/amd64` — macOS on Intel, community-tested
- `darwin/arm64` — macOS on Apple Silicon, native

Note: macOS ships two archives, not one universal binary. The Go binary is
architecture-specific while the embedded tools are universal, so a universal
atak would carry two full copies of the tools — about 45MB against a 25MB
target. Two 22MB archives stay under it.

The architecture of the atak process decides the architecture of every tool it
spawns: a universal child inherits the parent's slice, so an `atak-macos` built
for arm64 runs texconv, 7zz and compressonator-bc7e natively, and an x86-64
build runs all three under Rosetta 2. That is why `darwin/arm64` is a release
target rather than an optional extra — before it existed, every release user on
Apple Silicon was translated end to end.

The `-s -w` flags strip debug info. Final binaries should be under 25MB including
all embedded tools. With compressonator-bc7e on all three platforms, stripped
release sizes are roughly macOS ~22MB, Linux ~20MB, Windows ~11MB — still under
the ceiling, with macOS the tightest since its universal tools carry two slices
each. Track this if further binaries land.

## Cross-Platform Rules

These must be followed in every file or platform support silently breaks:

- **Never** use `/` as a path separator. Always `filepath.Join`.
- **Never** hardcode `~/.config`. Always `os.UserConfigDir()`.
- **Never** assume execute permissions need setting on Windows — `chmod` calls
  must be gated behind a build tag or `runtime.GOOS` check.
- All subprocess invocations via `exec.Command` use the extracted binary path
  from `EmbeddedTools` — never a hardcoded binary name.
- **Extension case:** Never assume `.dds` — always preserve the original file's
  extension case when writing output. Use `os.Rename` to match original case.
- **macOS:** `os.UserConfigDir()` returns `~/Library/Application Support` —
  no special handling needed, already correct via the stdlib.
- **macOS process management:** Same as Linux — `syscall.SysProcAttr{Setpgid: true}`
  and `syscall.Kill(-pid, syscall.SIGKILL)` work on Darwin. `process_linux.go`
  build tag should be `//go:build linux || darwin`.
- **compressonator-bc7e ships on all three platforms.** The macOS binary is
  built from source rather than taken from the fork's releases; see "Building
  compressonator-bc7e for macOS" under Embedded Binaries, and do not rebuild it
  with an arm64 ISPC. Availability is still expressed as
  `EmbeddedTools.CompressonatorPath == ""` rather than a `runtime.GOOS` test —
  mirrors how the lockfile / Job-Object process split is keyed off feature
  availability rather than raw OS checks, and it is what a future platform
  without a build would rely on.
