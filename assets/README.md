# Brand assets

Source images for the smyklot GitHub App. Kept here so they don't only live on one laptop.

## Files

| File | Size | Contents |
| --- | --- | --- |
| `smyklot-avatar.png` | 1024x1024 | Robot only. This is the image currently set as the GitHub App avatar. |
| `smyklot-avatar-768.png` | 768x768 | Same image, downscaled. |
| `smyklot-logo.png` | 1024x1024 | Robot plus the SMYKLOT wordmark. |
| `smyklot-logo-768.png` | 768x768 | Same image, downscaled. |

The 768px files are exact downscales of their 1024px masters, so either one works as a
starting point.

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
