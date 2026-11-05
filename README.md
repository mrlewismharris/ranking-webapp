# FreeCompare

Compare videos, images and text in pairs to rank your favourites.

## Run

Requires Go 1.26 or newer, plus FFmpeg and ffprobe for video uploads.

Copy `.env.example` to `.env`, adjust your settings, then run from the app's directory:

```sh
go run .
```

Open [localhost:8080](http://localhost:8080), or change the port with `ADDR=:3000` in `.env` and restart.

### Reset all app data

Stop the app, then run `clean.bat` on Windows or `bash clean.sh` in Bash to delete the database and uploaded files.
For custom storage, set `DATA_DIR` in your shell to match `.env` before cleaning.

## Keyboard controls

- **Left / Right:** play that clip and pause the other, or select an image/text choice.
- **Enter:** vote for the playing clip or selected image/text choice.
- **Up / Down:** adjust volume by 20 percentage points with an onscreen percentage.
- **Space:** no action.

## Features

- **Multiple game modes:** Tournament advances winners until the final rankings are decided. Normal compares every choice once, repeating one choice for odd counts. Endless repeats Normal with newly shuffled pairs after each pass.
- **Tournament rules:** Random pairs compete each round, with one repeated contender for odd counts advancing once if it wins either matchup; placement matches decide the top places.
- Create comparisons with 2–256 videos, images or text choices, plus optional descriptions and creator names.
- Add or replace media using file selection or drag and drop.
- Uploaded videos are prepared automatically for browser playback.
- Compare side by side with playback controls and an expanded media viewer.
- Resume saved progress and use Back to replace earlier votes.
- Browse tournaments on the homepage and share permanent links.
- Edit or delete your creations from the original browser.
- View your bracket, public scores and a ranked media slideshow after a tournament.
