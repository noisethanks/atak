# ATAK — Anomaly Texture Analysis Kit

> Texture compression and backup utility for S.T.A.L.K.E.R. Anomaly modlists.

![ATAK screenshot](atak.png)
![Compression results](results.png)

S.T.A.L.K.E.R. Anomaly and its modpacks represent a labor of love by hundreds of modders, culminating in a unique and memorable gaming experience. However, the ecosystem ships many texture assets uncompressed. On hardware with limited VRAM, this causes stuttering, hitching, and outright crashes during gameplay. ATAK compresses those textures to BCn block compression formats, dramatically reducing VRAM pressure with minimal visual difference.

Works with GAMMA, EFP, and any Anomaly-based modpack.

---

## Download

Grab the latest binary for your platform from [Releases](https://github.com/noisethanks/atak/releases):

| Platform | File |
|---|---|
| Linux x86-64 | `atak-vX.X.X-linux-x64.tar.gz` |
| Windows x86-64 | `atak-vX.X.X-windows-x64.zip` |
| macOS (Intel + Apple Silicon) | `atak-vX.X.X-macos.tar.gz` |

No installation required.

**Linux / macOS:**
```bash
tar -xzf atak-vX.X.X-linux-x64.tar.gz
chmod +x atak-linux
./atak-linux
```

**Windows:** Extract the zip, run `atak-windows.exe` in Windows Terminal or PowerShell.

---

## First time? Start here.

ATAK defaults to **Mod Output Mode** — a non-destructive workflow that outputs compressed textures to a separate MO2 mod folder instead of modifying your originals. This is the recommended approach.

**Setup:**
1. Launch ATAK and go to **Settings**
2. Set your MO2 `modlist.txt` path
3. Set output mod name (default: `ATAK`)
4. Go to **Backup Manager** and create a backup — still recommended as a safety net
5. Go to **Scan & Compress** and press **[r]** to compress everything
6. Add the output folder (e.g. `mods/ATAK/`) as a mod in MO2
7. Place it at the **top** of your load order and enable it

**To compress in-place instead** (replaces original files — make a backup first):
Disable Mod Output Mode in Settings. Then follow the backup-first workflow below.

**In-place workflow:**
1. Go to **Backup Manager** — create a backup first, always
2. Go to **Scan & Compress**
3. Press **[m]** to compress a single mod first — verify it looks right in game
4. If happy, press **[r]** to compress everything
5. If something looks wrong, restore from backup

---

## What it does

### Backup & Restore
- Create compressed LZMA archives of your full Anomaly mods directory
- Restore individual mods or your entire modlist from backup
- Verify archive integrity

### Mod Output Mode
Non-destructive compression that reads your MO2 `modlist.txt` to build a virtual filesystem — the same merged view MO2 presents to the game. Only the winning file for each texture path is compressed (respecting load order). Output goes to a single flat mod folder you add to MO2.

- Your original textures are never modified
- Incremental updates — rerun after adding mods, only new files are compressed
- Delete the output folder to force full recompression
- If scan shows 0 textures, check your modlist.txt path in Settings

### Scan & Compress
ATAK scans for uncompressed DDS textures and classifies them by type using filename patterns and directory paths. Only textures explicitly matched by a profile are compressed — nothing is touched blindly.

Default profiles cover the most common texture categories:

| Profile | Format | Detection method |
|---|---|---|
| Normal / bump maps | BC5 | `_bump`, `_normal`, `_nrm`, `_norm` suffixes |
| Sights / Reticles | BC3 | `scope_reticles/`, `bonus_sights/`, `*crosshair*` |
| Scope textures | BC7 | `*scope*diff*`, `*scope*bump*`, `*lens_bump*`, `*/textures/wpn/scope_*` |
| UI / Icons | BC3 | `textures/ui/` path |
| Diffuse / color | BC3 | `_diff`, `_base`, `_col`, `_d` suffixes |
| Weapon textures | BC3 | `textures/wpn/`, `textures/rwap/` paths |
| Character / hands | BC3 | `textures/act/`, `textures/MK/` paths |
| Sky textures | BC3 | `textures/sky/` path |
| Terrain / detail | BC3 | `textures/terrain/`, `textures/detail/` paths |
| Items | BC3 | `textures/items/`, `textures/item/`, `textures/usable_items/` paths |
| Particle / FX | BC3 | `textures/semitone/` path |

Already-compressed textures (~24,000 in a typical Anomaly install) are detected and skipped automatically.

**Unmatched textures are never touched.** Files that don't match any profile are shown in scan results but excluded from compression. Add patterns to `profiles.json` to include them.

---

## Compression formats

ATAK uses BCn block compression — a GPU-native format that decompresses in hardware with no performance cost. The tradeoff is a small, usually imperceptible quality loss during the compression step.

| Format | Field value | Alpha | Quality | Size | Linux GPU | Best for |
|---|---|---|---|---|---|---|
| BC1 | `BC1_UNORM` | No | Good | 0.5 bpt | Yes | Opaque diffuse, environment |
| BC3 | `BC3_UNORM` | Yes | Good | 1 bpt | Yes | UI, general alpha textures |
| BC5 | `BC5_UNORM` | No | Excellent | 1 bpt | Yes | Normal maps only |
| BC7 | `BC7_UNORM` | Yes | Excellent | 1 bpt | No (CPU) | High quality diffuse, weapons |

*bpt = bytes per texel*

**BC5 is required for normal maps** — using BC3 on normal maps produces incorrect lighting. Do not change the Normal Maps profile format.

**BC7 on Linux** is CPU-only — no GPU acceleration available. Expect 40-60 minutes for large jobs. For faster Linux compression, use BC3 for all profiles (the default). Quality difference is minimal at normal viewing distances.

**Mip chains:** ATAK resolves mip generation per file. World-texture profiles (`generateMips: true`) always get a full chain built with cubic filtering, letting the engine load lower-resolution versions for distant objects and reducing effective VRAM further. Fixed-size art profiles (`generateMips: false`) follow the source instead — a texture that shipped with mips keeps them, one that didn't stays single-level. This matters for flares and scope reticles: many ship a mip chain and flattening them makes them alias and read as the wrong shade in-game. To force the old always-strip behavior for `generateMips: false`, enable `stripMipsWhenDisabled` in Settings. Keep your in-game texture quality setting at High — lowering it on top of BCn compression will reduce visual quality unnecessarily.

---

## Configuration

Config lives at:
- **Linux:** `~/.config/atak/`
- **Windows:** `%AppData%\atak\`
- **macOS:** `~/Library/Application Support/atak/`

### config.json

```json
{
  "modsDir": "/path/to/Anomaly/mods",
  "backupDir": "/path/to/backups",
  "workerCount": 1,
  "backupThreads": 4,
  "backupLevel": 6,
  "scanExclusions": [".*", "downloads", "Downloads", "G.A.M.M.A. UI"],
  "modOutputMode": true,
  "modOutputName": "ATAK",
  "modlistPath": "/path/to/MO2/profiles/Default/modlist.txt",
  "stripMipsWhenDisabled": false
}
```

- `workerCount` — concurrent texconv processes (compression only). Each worker pegs one CPU core. Default: 1
- `backupThreads` — 7-Zip thread count for backup/restore operations. Default: half your CPU threads
- `backupLevel` — 7-Zip compression level 1-9. Default: 6
- `scanExclusions` — directories to skip during scan. A plain name (`downloads`, `.*`) matches a directory or mod name anywhere; a path pattern (`*/textures/ui/SquareDOV`) matches a nested directory, using the same pattern syntax as the profile lists above. The matched directory and everything under it is skipped
- `modOutputMode` — non-destructive output mode. Default: true
- `modOutputName` — output mod folder name. Default: "ATAK"
- `modlistPath` — path to MO2 `modlist.txt`. Required when `modOutputMode` is true
- `stripMipsWhenDisabled` — when true, profiles with `generateMips: false` strip mips entirely, dropping any chain the source shipped. When false (default), such a source keeps its chain (see `generateMips` below). Never affects `generateMips: true`. Toggle in Settings ("Strip Mips When Disabled")

**Finding your modlist.txt:**
- Usually at `<MO2 install>/profiles/<Profile Name>/modlist.txt`
- In MO2: click the profile dropdown → "Open Profile Folder"

### profiles.json

Controls which textures get compressed and how. Created on first run from embedded defaults. Edit freely.

```json
{
  "minFileSizeBytes": 1024,
  "excludePatterns": ["fx_sun*", "fx_*", "lut_*", "*#small*", "*cube#*"],
  "profiles": [
    {
      "name": "Normal Maps",
      "format": "BC5_UNORM",
      "generateMips": true,
      "maxTextureSize": 0,
      "patterns": ["*_bump.*", "*_normal.*"],
      "exclude": []
    }
  ]
}
```

**Top-level fields:**
- `minFileSizeBytes` — skip files smaller than this. Default: 1024
- `excludePatterns` — glob patterns never compressed regardless of profile match. Matched against the filename, or against the full path when the pattern contains `/`. These run *before* profile matching, so an entry here always beats a profile pattern

**Per-profile fields:**
- `name` — display name in scan results
- `format` — compression format. See table above
- `generateMips` — mip-chain **policy**, not an on/off switch. `true` forces a full chain — use for world textures (diffuse, normals, weapons, terrain, sky) that minify with distance. `false` **preserves the source's own choice**: a source that shipped mips (many flares, scope reticles) keeps its chain, one that didn't (most flat UI art) stays single-level. Set the `stripMipsWhenDisabled` config option to make `false` strip unconditionally instead
- `maxTextureSize` — cap output resolution. `0` = no limit. Set to `1024` on sky/terrain profiles for 4GB VRAM cards. Textures smaller than this value are never upscaled
- `patterns` — glob patterns matched against filename or full path
- `exclude` — optional. A file matching this profile's `patterns` **and** its `exclude` is declined by this profile, and matching continues with later profiles. Use it to route exceptions elsewhere — Normal Maps declines `*scope*bump*` so scope lens bumps fall through to Scope Textures (BC7) instead of being flattened to two-channel BC5. To drop a file outright, use the top-level `excludePatterns` instead

**Pattern syntax:**
- `*` matches any characters except `/`; `?` matches a single character except `/`
- Patterns without `/` match filename only: `*_bump.*` matches `rock_bump.dds`
- Patterns with `/` match full path: `*/textures/wpn/scope_*` matches `gamedata/textures/wpn/scope_30mm.dds`
- **A trailing `/*` is recursive** — it covers the whole subtree, not just direct children. `*/textures/ui/SquareDOV/*` also excludes `textures/ui/SquareDOV/nested/file.dds`. A bare trailing `/` means the same thing, so `*/textures/ui/SquareDOV/` is equivalent
- Matching is case-insensitive on all platforms
- **Order matters — first match wins.** Put specific patterns before general ones

Community profiles available in the `profiles/` directory in the repository:
- `default.json` — conservative BC3 defaults (same as embedded)
- `quality.json` — BC7 for weapons and characters (Windows GPU recommended)

---

## Performance

**Note on BC7:** BC7 GPU acceleration is not available on Linux — CPU-only, ~40-60 min for large jobs. Windows benefits from DirectX GPU acceleration. For faster Linux compression, keep all profiles at BC3 (the default).

Benchmarks and feedback welcome — open a GitHub issue or post in the community Discord.

---

## Compression quality

Default profiles use BC3 for all textures except Normal Maps (BC5) and Scope Textures (BC7). BC3 produces minimal visible quality loss at typical Anomaly viewing distances and compresses quickly on all platforms.

Scope Textures is the one BC7 profile in the defaults. Scope lens textures often pack reflection, gloss, and specular data across all four RGBA channels, so BC5 would discard blue and alpha and BC3 would band the gradients. It covers a small number of files, so the CPU cost on Linux stays bounded — see [Compression formats](#compression-formats) for the BC7 caveat.

For higher quality weapon and character textures, copy `profiles/quality.json` from the repository to your config directory — this uses BC7 for weapons and characters. Recommended for Windows users with GPU acceleration.

For 4GB VRAM cards, set `"maxTextureSize": 1024` on Sky and Terrain profiles in `profiles.json` to reduce VRAM usage further beyond BCn compression.

---

## Known limitations

- BC7 GPU acceleration not available on Linux — CPU fallback, slow on large jobs
- Cancelling a compression job deletes the in-progress file — originals untouched
- 7-Zip backup progress is sparse on large solid archives — the archive is growing even when the progress bar appears stuck
- If the app crashes during backup/restore on Linux, run `pkill 7zz` if you notice high CPU/RAM usage afterwards
- Mod Output Mode without a valid modlist.txt will show 0 textures — check Settings if this happens

---

## Building from source

Requires Go 1.21+.

```bash
git clone https://github.com/noisethanks/atak
cd atak
go build -o atak-linux .
```

Release build:
```bash
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
  -ldflags="-s -w -X main.version=v0.2.0" \
  -o atak-linux .
```

---

## GAMMA

[S.T.A.L.K.E.R. GAMMA](https://www.stalkergamma.com/) is a large modpack
for S.T.A.L.K.E.R. Anomaly maintained by Grok. Join the community on
[Discord](https://discord.com/invite/stalker-gamma).

---

## Attributions

- [texconv (Texconv-Custom-DLL)](https://github.com/matyalatte/Texconv-Custom-DLL) — MIT — matyalatte
- [7-Zip](https://www.7-zip.org) — LGPL v2.1 — Igor Pavlov
- [Bubble Tea](https://github.com/charmbracelet/bubbletea) — MIT — Charmbracelet
- [Bubbles](https://github.com/charmbracelet/bubbles) — MIT — Charmbracelet
- [Lip Gloss](https://github.com/charmbracelet/lipgloss) — MIT — Charmbracelet

The optional compressonator-bc7e compression backend additionally
incorporates:

- [AMD Compressonator](https://github.com/GPUOpen-Tools/compressonator) — MIT — © 2024 Advanced Micro Devices, Inc.; © 2004-2006 ATI Technologies Inc.
- [nlohmann/json](https://github.com/nlohmann/json) — MIT — © 2013-2017 Niels Lohmann (linked into Compressonator)
- [bc7e.ispc from richgel999/bc7enc_rdo](https://github.com/richgel999/bc7enc_rdo) — Apache License 2.0 — © 2018-2021 Binomial LLC (Richard Geldreich, Jr.)

> This software incorporates bc7e.ispc from richgel999/bc7enc_rdo,
> © Richard Geldreich / Binomial LLC, licensed under the Apache License,
> Version 2.0.

Full license text for every third-party component ships in
`licenses/compressonator-bc7e/` and is also available in-app via the About
screen.

---

## Support

If ATAK saved your playthrough, consider supporting development:

**[noisethanks.com/support](https://noisethanks.com/support)**

Made by [mrchocolate](https://github.com/noisethanks)
