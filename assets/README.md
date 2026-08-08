# Brand assets

Source images for the smyklot GitHub App. Kept here so they don't only live on one laptop.

One image - the robot, no wordmark - exported at every size a platform is likely to ask for.

## Layout

| Directory | Contents                                                   |
| --------- | ---------------------------------------------------------- |
| `master/` | The approved native render and its large exports           |
| `png/`    | Exact-pixel small sizes: 256, 192, 128, 96, 64, 48, 32, 16 |
| `retina/` | `@2x` / `@3x` aliases for the small sizes                  |

All PNG, RGB, square.

## Which file to use

`master/robot-avatar-master-native-1254x1254.png` is the approved generated image and the only
file with native detail. Use it wherever the platform accepts an arbitrary size.

Everything else is derived from it. The 512, 768 and 1024 exports are downscales. The 2048 and
4096 exports are Lanczos upscales kept for convenience - they are larger, not sharper, so reach
for them only when something demands those dimensions.

The `retina/` files are aliases, not separate renders. Ten of the twelve are byte-identical to a
file in `png/` or `master/` (`256@2x` is the 512, `128@2x` is the 256, and so on). Only `48@3x`
(144px) and `128@3x` (384px) are sizes that exist nowhere else. The directory is here so a build
that expects `@2x` naming can point straight at it.

## Which one is live

The app serves its avatar from `https://avatars.githubusercontent.com/in/1197525`. To change it,
upload a file under Display information on the app's settings page - there is no API for it.
GitHub takes the native master fine.

## Verifying

`SHA256SUMS.txt` ships with the export and covers every image:

```sh
cd assets && shasum -a 256 -c SHA256SUMS.txt
```

The manifest's original `README.md` line was dropped when this file replaced the one that came in
the package. Image hashes are untouched.

## Origin

Generated 2026-08-08, replacing the first avatar and the SMYKLOT wordmark logo that had been here
since 2025-03-30. The new set has no wordmark variant, so the logo files are gone - `git log` has
them if they are ever wanted back.

The PNGs are straight from the generator, unoptimized, about 18 MB for the set. Left as-is on
purpose so these stay the originals. Squeeze them later if the repo size ever matters.
