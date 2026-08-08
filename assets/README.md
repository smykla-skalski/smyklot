# Brand assets

Source images for the smyklot GitHub App. Kept here so they don't only live on one laptop.

One image - the robot, no wordmark - at one file per pixel size. Nothing here duplicates anything else here.

## Layout

`robot-avatar-master-native-1254x1254.png` is the approved generated image. It is the only file with native detail, and the only one worth treating as a source. Use it wherever the platform accepts an arbitrary size.

`png/` holds thirteen downscales of it: 16, 32, 48, 64, 96, 128, 144, 192, 256, 384, 512, 768 and 1024 px. All PNG, RGB, square.

## Deriving anything else

Every file in `png/` reproduces pixel for pixel as a Pillow Lanczos resample of the native, with zero differing subpixels. Checked, not assumed. So any size you need and don't see is one line away:

```sh
python3 -c "from PIL import Image; im = Image.open('robot-avatar-master-native-1254x1254.png'); im.resize((320, 320), Image.LANCZOS).save('robot-avatar-320x320.png')"
```

The pixels come back identical. The encoded PNG bytes may not - that depends on encoder settings, so don't expect a byte-for-byte match against the committed files.

Use Pillow specifically. ImageMagick's Lanczos is a different kernel and does not reproduce these files.

## Retina naming

The export shipped a `retina/` directory of `@2x` / `@3x` aliases. Ten of its twelve files were byte-identical copies of files already in the set, so it is gone. If something expects that naming, point it at the pixel size instead:

| Alias  | @2x    | @3x    |
| ------ | ------ | ------ |
| `16`   | 32 px  | 48 px  |
| `32`   | 64 px  | 96 px  |
| `48`   | 96 px  | 144 px |
| `64`   | 128 px | 192 px |
| `128`  | 256 px | 384 px |
| `256`  | 512 px | 768 px |

The two sizes that existed only under retina names, 144 and 384, are in `png/` as ordinary files.

## What was dropped

The export also shipped 2048 and 4096 px versions. Both were upscales of the 1254 native - larger, not sharper, carrying no detail the native lacks - and together they were roughly three quarters of the package's 18 MB. They are not worth committing when the native renders better and anything larger can be resampled on demand.

## Which one is live

The app serves its avatar from `https://avatars.githubusercontent.com/in/1197525`. To change it, upload a file under Display information on the app's settings page - there is no API for it. GitHub takes the native master fine.

## Verifying

```sh
cd assets && shasum -a 256 -c SHA256SUMS.txt
```

The manifest was regenerated after this reorganization. Every hash in it is unchanged from the one the export shipped - the files moved and were pruned, none were re-encoded.

## Origin

Generated 2026-08-08, replacing the first avatar and the SMYKLOT wordmark logo that had been here since 2025-03-30. The new set has no wordmark variant, so the logo files are gone - `git log` has them if they are ever wanted back.

The PNGs are straight from the generator, unoptimized, about 4.6 MB for the set. Left as-is on purpose so these stay the originals. Squeeze them later if the repo size ever matters.
