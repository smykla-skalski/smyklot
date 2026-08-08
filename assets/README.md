# Brand assets

Source images for the smyklot GitHub App. Kept here so they don't only live on one laptop.

One image - the robot, no wordmark - as `smyklot-avatar-<N>.png`, where `N` is the pixel size. Square, so one number says it. Nothing here duplicates anything else here.

## The files

`smyklot-avatar-1254.png` is the approved generated image. It is the only file with native detail and the only one worth treating as a source. Use it wherever the platform accepts an arbitrary size.

The other eight are downscales of it: 16, 32, 48, 64, 128, 256, 512 and 1024 px. All PNG, RGB, square.

## Deriving anything else

Every downscale reproduces pixel for pixel as a Pillow Lanczos resample of the 1254, with zero differing subpixels. Checked, not assumed. So any size you need and don't see is one line away:

```sh
python3 -c "from PIL import Image; im = Image.open('smyklot-avatar-1254.png'); im.resize((320, 320), Image.LANCZOS).save('smyklot-avatar-320.png')"
```

The pixels come back identical. The encoded PNG bytes may not - that depends on encoder settings, so don't expect a byte-for-byte match against the committed files.

Use Pillow specifically. ImageMagick's Lanczos is a different kernel and does not reproduce these files.

## Retina naming

The export shipped a `retina/` directory of `@2x` / `@3x` aliases, ten of whose twelve files were byte-identical copies of files already in the set. It is gone. Anything expecting that naming should ask for the pixel size instead: `@2x` of 16, 32, 64, 128, 256 and 512 are all present as 32, 64, 128, 256, 512 and 1024. Of the `@3x` sizes only 48 survives, as `16@3x`. Derive the rest with the one-liner above if something ever needs them.

## What was dropped

The export shipped 2048 and 4096 px versions. Both were upscales of the 1254 native - larger, not sharper, carrying no detail the native lacks - and together they were roughly three quarters of the package's 18 MB. Not worth committing when the native renders better and anything larger can be resampled on demand.

The 96, 144, 192, 384 and 768 px sizes came out too. They were the ×3 steps between the sizes anyone actually asks for, and each is a one-line resample away.

## Which one is live

The app serves its avatar from `https://avatars.githubusercontent.com/in/1197525`. To change it, upload a file under Display information on the app's settings page - there is no API for it. GitHub takes the 1254 fine.

## Verifying

```sh
cd assets && shasum -a 256 -c SHA256SUMS.txt
```

The manifest was regenerated after the files were renamed and pruned. Every hash in it is unchanged from the one the export shipped - nothing was re-encoded.

## Origin

Generated 2026-08-08, replacing the first avatar and the SMYKLOT wordmark logo that had been here since 2025-03-30. The new set has no wordmark variant, so the logo files are gone - `git log` has them if they are ever wanted back.

The PNGs are straight from the generator, unoptimized, about 3.6 MB for the set. Left as-is on purpose so these stay the originals. Squeeze them later if the repo size ever matters.
