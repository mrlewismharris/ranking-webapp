# Crown

A server-rendered ranking tournament app built with Go, `html/template`, SQLite (pure Go `modernc.org/sqlite`), and small vanilla JavaScript enhancements. No frontend build step or framework.

## Run

Requires Go 1.26 or newer.

```sh
go run .
```

Open http://localhost:8080. Optional configuration:

```sh
ADDR=127.0.0.1:3000 DATA_DIR=./data go run .
```

Build a standalone executable (templates and static files are embedded):

```sh
go build -o bin/crown .
./bin/crown
```

## Features

- Named tournaments with descriptions, optional public creator names, and 2–256 contenders.
- Creator-only edit links and deletion, authorized by the original browser cookie. A “Your creations” section lists owned tournaments.
- Text, JPG, PNG, GIF, WebP, or MP4 contenders. Media titles are optional.
- Individual media attachment, replacement, multi-file selection, and drag-and-drop uploads.
- Files stored in `data/uploads/`, with metadata and paths in `data/bracket.db`.
- Media endpoint supports byte ranges for video seeking. MP4 playback requires a browser-supported codec (H.264 is a widely supported choice); no transcoding is performed.
- Permanent tournament links, with an independent randomized bracket for each visitor.
- Anonymous, HttpOnly, SameSite cookie identifies the browser for one year. SQLite saves every vote. Clearing cookies or changing browsers creates a separate identity.
- Home page lists your started and completed tournaments. Tournament pages offer continue, view results, or start fresh. Restart replaces current progress; the last completed public ballot remains until the next completion.
- Full-viewport side-by-side cards with a tiny exit/logo link, crown voting, hover-loop video previews, touch play/pause, and a full-viewport media lightbox.
- Responsive layout, keyboard focus styling, transition animations, and reduced-motion support.
- Stale vote protection prevents duplicate submissions from advancing another matchup.

## Tournament rules

The initial contenders are shuffled once and each round’s matchups are persisted before voting. Winners retain their bracket order for the next round.

For each odd round (including three contenders), a random entrant plays a second matchup against the unpaired entrant. A contender who wins either matchup advances **once**, even if they win both. This means a repeated entrant can advance despite losing their other matchup.

At four contenders, there are two semifinals, a third/fourth-place match between the losers, and a first/second-place final between the winners. At three, the odd-round rule still applies: two distinct winners play a final; if one contender wins both matches, the other two play for silver and bronze. If an earlier odd round reduces directly to two finalists, the eliminated contenders play placement matches before the final. Two-item tournaments have only first and second place. These rules ensure a unique podium and termination for all supported sizes.

## Check

```sh
go test ./...
go test -race ./...
go vet ./...
```

Tests cover all sizes from 2 through 256, odd-round pairing, exact four-player placements, invalid/stale votes, persistence, restart, HTML rendering, media range requests, and cross-origin rejection.

## Storage and deployment notes

Keep the entire data directory on persistent storage. Back up SQLite using a SQLite-aware backup, or stop the app before copying the database and uploaded files together. No accounts, moderation, transcoding, or upload garbage collection are included. Abandoned, replaced, and deleted tournament uploads remain on disk. Deleting a tournament revokes public access to its media; the original uploader can still access unclaimed uploads. Creator drafts themselves are kept in memory until creation.

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
