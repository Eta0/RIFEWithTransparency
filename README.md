# RIFE with Transparency

Simple tool wrapping [Practical-RIFE](https://github.com/hzwer/Practical-RIFE) to interpolate frame animations
with transparency and reassemble them into animated PNGs with around twice as many frames.

## Usage

- Run `RifeWithTransparency in.gif out.png` to double the frames in `in.gif`, saving the result as an APNG named `out.png`.
- Animated WebP files may also be used as input.
- A third argument can be given to specify a *matte colour*;
transparent pixels that erroneously become opaque will take on this colour,
and semi-transparent pixels may blend against this colour during interpolation.
The default matte colour is `#36393F`.

## Algorithm

RIFE with Transparency splits a frame animation with transparency into an opaque sequence of frames,
plus a sequence of black and white frames corresponding to the original alpha channel.
Both are interpolated in parallel, and then the interpolated alpha channel is reapplied to the interpolated opaque frame sequence,
and assembled into an animated PNG with transparency.

Additionally, RIFE with Transparency adds a copy of the start frame to interpolate against at the end
so that the interpolation produces a smooth loop.
The final frame of the APNG output can be removed if this is not desired.

## PATH Dependencies

Running `RifeWithTransparency` requires the following programs to be accessible under the specified names on your PATH:

1. [Practical-RIFE](https://github.com/hzwer/Practical-RIFE) as `rife`
2. [ImageMagick](https://imagemagick.org/index.php) as `magick`
3. [APNG Assembler](https://apngasm.sourceforge.net/) as `apngasm64`

Recommended:

- [`apng2gif`](https://apng2gif.sourceforge.net/) can be used on the resulting APNG to convert it back to a GIF.
  Partial transparency will be lost.

## License

RIFE with Transparency is free and open-source software provided under the [zlib license](https://opensource.org/licenses/Zlib).