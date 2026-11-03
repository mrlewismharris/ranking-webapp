# FreeCompare

A server-rendered ranking tournament app built with Go, `html/template`, SQLite (pure Go `modernc.org/sqlite`), and small vanilla JavaScript enhancements. No frontend build step or framework.

## Run

Requires Go 1.26 or newer. Video uploads also require FFmpeg (with the libx264 encoder) and ffprobe on the server PATH. Images and text work without them. Set FFMPEG_PATH and FFPROBE_PATH to executable paths if they are installed elsewhere.

```sh
go run .
```

Open http://localhost:8080. On a fresh checkout, copy `.env.example` to `.env` in the app's root directory:

```powershell
Copy-Item .env.example .env
```

Or in Bash:

```sh
cp .env.example .env
```

Edit `.env` to choose a port and storage directory:

```dotenv
ADDR=:3000
DATA_DIR=./data
```

Run `go run .` from that directory and open http://localhost:3000. Restart after changing `.env`. `ADDR=127.0.0.1:3000` restricts connections to your own computer. `.env.example` also documents optional `FFMPEG_PATH` and `FFPROBE_PATH` settings, comments and quoted paths. Local `.env` files and variants such as `.env.local` are ignored by Git; `.env.example` is committed as the shared template.

The app reads `.env` from the **current working directory**, including when using the compiled executable. A missing `.env` uses the existing defaults; an unreadable or malformed file stops startup with an error. Existing environment variables override `.env`, so shell or deployment configuration still works:

```sh
ADDR=127.0.0.1:3000 DATA_DIR=./data go run .
```

If you previously set `$env:ADDR` in PowerShell, run `Remove-Item Env:ADDR` to let `.env` control the address in that terminal.

Build a standalone executable (templates and static files are embedded):

```sh
go build -o bin/freecompare .
./bin/freecompare
```

## Features

- Named tournaments with descriptions, optional public creator names, and 2–256 contenders.
- Creator-only edit links and deletion, authorized by the original browser cookie. A “Your creations” section lists owned tournaments.
- Text, JPG, PNG, GIF, WebP, or video contenders (MP4, MOV, MKV, WebM, AVI, M4V, MPEG, WMV, FLV, TS/MTS/M2TS, 3GP/3G2 and OGV). Video contents are validated with ffprobe; codec support depends on the server FFmpeg build. Media titles are optional.
- Individual media attachment, replacement, multi-file selection, and drag-and-drop uploads.
- Files stored in `data/uploads/`, with metadata and paths in `data/bracket.db`.
- Video uploads are converted to browser-compatible H.264 MP4 with fast-start playback. Existing 8-bit YUV 4:2:0 H.264 video is remuxed without re-encoding (no added video quality loss); AAC audio is also copied. Other video uses libx264, CRF 16, slow preset and YUV 4:2:0; other audio becomes 256 kbps AAC. Re-encoding is high quality, but not mathematically lossless: changing codecs, bit depth, chroma format or audio encoding can introduce loss. No resolution reduction or forced frame rate is applied; odd dimensions are padded by one pixel. The first non-cover-art video and first audio stream are kept; additional tracks/subtitles are omitted. HDR inputs are not tone-mapped. Existing uploads are unchanged.
- Conversion happens before an upload appears in the contender list, with two jobs at a time and a 15-minute deadline (including queue time). Failed conversions remove their files. Converted files can be larger than the source. The 64 MB limit applies to the uploaded source.
- Media endpoint supports byte ranges for video seeking.
- Black terminal theme with monospace typography, green accents and square controls.
- Permanent tournament links, with an independent randomized bracket for each visitor.
- Anonymous, HttpOnly, SameSite cookie identifies the browser for one year. SQLite saves every vote. Clearing cookies or changing browsers creates a separate identity.
- Home page shows all tournaments to every visitor, newest first, alongside your started and completed tournaments and your creations. New tournaments appear on the next page load. Tournament pages offer continue, view results, or start fresh. Restart replaces current progress; the last completed public ballot remains until the next completion.
- Full-viewport side-by-side cards with a tiny exit/logo link, choice buttons, exclusive video playback, touch play/pause, and a full-viewport media lightbox.
- Start as Tournament, Start Normal (one pass), or Start Endless Mode (reshuffled passes). All modes save your progress.
- Responsive layout, keyboard focus styling, transition animations, and reduced-motion support.
- Stale vote protection prevents duplicate submissions from advancing another matchup.

## Tournament rules

The initial contenders are shuffled once and each round’s matchups are persisted before voting. Winners retain their bracket order for the next round.

For each odd round (including three contenders), a random entrant plays a second matchup against the unpaired entrant. A contender who wins either matchup advances **once**, even if they win both. This means a repeated entrant can advance despite losing their other matchup.

At four contenders, there are two semifinals, a third/fourth-place match between the losers, and a first/second-place final between the winners. At three, the odd-round rule still applies: two distinct winners play a final; if one contender wins both matches, the other two play for silver and bronze. If an earlier odd round reduces directly to two finalists, the eliminated contenders play placement matches before the final. Two-item tournaments have only first and second place. These rules ensure a unique podium and termination for all supported sizes.

## Normal and Endless modes

Choose a mode on the tournament page. **Start Normal** shuffles all contenders into random pairs and finishes after that single pass, without elimination or placement matches. Each contender appears once; an odd count repeats one randomly selected contender to pair with the last one. These modes support images and text as well as videos.

**Start Endless Mode** uses the same pairing rules, then shuffles all contenders into a new pass when the current one ends. Draws minimize repeated opponents from the previous pass; with only two or three contenders some repeated pairs are unavoidable. Progress resumes at the saved pair and pass. Endless saved state retains only the current pass; individual votes remain in SQLite.

Normal and Endless choices are recorded but do not contribute to the public tournament scoreboard or replace your last completed tournament result. Starting any mode replaces your current in-progress session for that tournament.

## Comparison keyboard controls

- **Left / Right arrow:** choose the left / right contender.
- **Space:** switch the playing video, pausing the previous clip and keeping its position. If there is just one video, Space resumes it.
- **Up / Down arrow:** raise / lower volume by 20 percentage points, clamped to 0–100%, with a small temporary percentage display. Changing volume also unmutes the clip unless the volume is zero.

The left video starts first (or the only video in a mixed pair). Only one clip plays at a time. Click or tap a video to pause or resume it. Shortcuts also work in the expanded media view, where Space switches the expanded clip. Seek sliders retain their normal arrow-key behavior when focused. Sound settings carry between matchups in the same browser. Initial playback is muted; browsers may also require muted autoplay before a user interaction.

## Reset all app data

Stop the app, then run the script for your platform:

```sh
bash clean.sh
```

```bat
clean.bat
```

Both permanently remove `data/bracket.db`, its SQLite sidecar files, and `data/uploads/`, including all tournaments, progress, votes and uploaded files. The app recreates empty storage on its next start. Other files in the data directory are preserved. The default data directory is relative to the script, so the scripts can be launched from another directory. The cleanup scripts do not read `.env`. If you customize `DATA_DIR` in `.env`, set that same environment variable when cleaning; relative custom paths resolve from your current working directory, as they do for the app. For example, in PowerShell, use `$env:DATA_DIR = './custom-data'` before running `clean.bat`.

## Check

```sh
go test ./...
go test -race ./...
go vet ./...
```

When FFmpeg and ffprobe are available, tests also convert real MOV, WebM, AVI, MKV and MP4 fixtures, verify lossless H.264 stream copy, test converted-video ranges, and check invalid video cleanup. These integration tests skip when the tools are absent.

Tests cover all sizes from 2 through 256, odd-round pairing, exact four-player placements, invalid/stale votes, persistence, restart, HTML rendering, media range requests, and cross-origin rejection.

## Storage and deployment notes

Keep the entire data directory on persistent storage. Back up SQLite using a SQLite-aware backup, or stop the app before copying the database and uploaded files together. No accounts, moderation, or upload garbage collection are included. Abandoned, replaced, and deleted tournament uploads remain on disk. Deleting a tournament revokes public access to its media; the original uploader can still access unclaimed uploads. Creator drafts themselves are kept in memory until creation.

Use an HTTPS reverse proxy for public hosting. Uploads are limited to 64 MB per file, but this initial version does not impose user quotas or rate limits. The app defaults to listening on all interfaces at port 8080; set `ADDR=127.0.0.1:8080` to bind only locally.

## Creator ownership and editing

The creator’s anonymous cookie ID is stored with new tournaments. `/t/{id}/edit` is a stable edit permalink, but knowing the link does not grant access: viewing, saving, and deleting all require the matching cookie. Creator names are display labels, not authentication. Losing the cookie means losing edit access. Pre-upgrade tournaments remain playable and gain scoreboards, but have no owner because the original app never recorded one; ownership is deliberately not guessed from participants.

Editing the tournament name, description, or creator name preserves games and public scores. Any contender change (including a title, media, order, addition, or removal) requires the reset checkbox and atomically removes all games, vote records, and results for that tournament. The public permalink remains the same. Optimistic version checks reject stale edit pages, and unique run IDs reject votes from old brackets. Deletion requires an explicit checkbox and removes the tournament and its dependent database rows.

## Public scoreboard

Each accepted head-to-head choice records tournament, anonymous voter ID, run ID, revision, both contenders, winner, and timestamp. Individual identifiers are never displayed publicly. Existing saved match histories are backfilled during migration (their original vote timestamps were not stored, so migrated rows use the migration timestamp).

The results screen lists every contender, including zero-score entries, with total points, wins/appearances, win rate, and champion picks. Only each browser’s latest **completed** run contributes; unfinished runs do not count, and replays replace rather than accumulate ballots. Equal point totals share a rank.

For each completed run, non-podium entrants earn one point per match win. Let `B` be that run’s highest individual win count. Podium scores replace win points: fourth = `B + 1`, third = `B + 2`, second = `B + 3`, first = `B + 4`. This guarantees placement order even with repeated entrants and consolation matches. Points are summed across the latest completed ballots. Win rate offers an additional view of performance per appearance; random draws still introduce noise. Cookie identities represent browsers, not verified unique people, so using another browser or clearing cookies can create another ballot.

## Personal bracket and media countdown

Completed results include a scrollable bracket tree with every saved matchup, highlighted picks, and connections between rounds. Dashed connections show entrants moving into placement matches after losing. Repeated entrants in odd rounds appear in both of their matches. This is reconstructed from saved histories, so existing completed tournaments also work without restarting.

If at least one contender has an image or video, **Watch your countdown** opens a full-screen slideshow. Non-podium contenders are ordered by the round in which they were eliminated; tied groups are shuffled deterministically for that completed run. Fourth, third, second, and first place follow in that order. Text-only contenders are skipped while the remaining media retain their original placements. If the champion is text-only, the highest-ranked media contender ends the slideshow.

Previous/next buttons let you browse manually. **Autoplay Slideshow** displays each loaded picture for three seconds and advances videos after one complete playback. Enabling autoplay on a video restarts that clip. With autoplay off, videos loop. Autoplay stops after the highest-ranked media item; it does not wrap back to last place. Videos begin muted, expose native controls, and retain your mute setting between slides. Closing the viewer stops media and timers; backgrounding the page pauses playback/timers.
