# Brand assets

Source images for the smyklot GitHub App. Kept here so they don't only live on one laptop.

## Files

Two images, each at four sizes.

| Image  | Master               | Downscales             | Contents                                                              |
| ------ | -------------------- | ---------------------- | --------------------------------------------------------------------- |
| Avatar | `smyklot-avatar.png` | `-768`, `-512`, `-256` | Robot only. This is the image currently set as the GitHub App avatar. |
| Logo   | `smyklot-logo.png`   | `-768`, `-512`, `-256` | Robot plus the SMYKLOT wordmark.                                      |

Masters are 1024x1024. Every smaller file is a plain ImageMagick resize of its master, so
pick whichever size fits and don't worry about which one is canonical. The wordmark stays
legible down to 256px.

The 512 and 256 files were generated with the PNG timestamp chunk excluded, which makes them
reproducible - rerun this and you get the committed file back byte for byte:

```sh
magick smyklot-avatar.png -resize 256x256 \
  -define png:exclude-chunk=date,time smyklot-avatar-256.png
```

Drop that `-define` and the pixels still match, but PNG stamps the encode time into the file
and the bytes come out different on every run. The 768s predate this and carry a timestamp.

## Which one is live

`smyklot-avatar.png` (the wordmark-free version). The app serves it from
`https://avatars.githubusercontent.com/in/1197525`. To change it, upload a new file under
Display information on the app's settings page - there is no API for it.

## Origin

Generated 2025-03-30, the same day the GitHub App was registered. They sat untracked in a
gitignored scratch directory of an unrelated repo until this commit.

The PNGs are unoptimized straight from the generator - roughly 1.2 MB each for flat art that
should compress far smaller. Left as-is on purpose so these stay the originals. Squeeze them
later if the repo size ever matters.
