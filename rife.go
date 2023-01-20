package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
)

func main() {
	errorLogger := log.New(os.Stderr, "", 0)
	if len(os.Args) != 3 && len(os.Args) != 4 {
		errorLogger.Fatal("usage: " + os.Args[0] + " input.gif output.png [#matte]")
	}
	source, err := filepath.Abs(os.Args[1])
	if err != nil {
		errorLogger.Fatal("error recognizing input path:\n  ", err)
	}
	if _, err = os.Stat(source); err != nil {
		errorLogger.Fatal("error opening input file:\n  ", err)
	}
	dest, err := filepath.Abs(os.Args[2])
	if err != nil {
		errorLogger.Fatal("error recognizing output path:\n  ", err)
	}
	var background string
	if len(os.Args) == 4 {
		background = os.Args[3]
	} else {
		background = "#36393F"
	}

	frameCount, err := interpolate(source, dest, background)
	if err != nil {
		errorLogger.Fatal(err)
	}

	fmt.Printf("%s : %d frames -> %d frames\n", os.Args[1], frameCount, frameCount*2+1)
}

func interpolate(source, dest, background string) (uint64, error) {
	// Interpolates the file at path `source`, outputting at path `dest`, with an intermediate matting colour specified by `background`.

	// Set up temporary directory structure

	dir, err := os.MkdirTemp("", "rife-interpolation-*")
	if err != nil {
		return 0, fmt.Errorf("error creating temporary directory:\n  %s", err)
	}
	defer func(path string) { _ = os.RemoveAll(path) }(dir)

	if err != nil {
		return 0, fmt.Errorf("error opening temporary directory:\n  %s", err)
	}

	frameDir := path.Join(dir, "Frames")
	alphaDir := path.Join(dir, "Alpha")
	interpolatedFrameDir := path.Join(dir, "IFrames")
	interpolatedAlphaDir := path.Join(dir, "IAlpha")
	mergedDir := path.Join(dir, "Merged")

	for _, childDir := range []string{frameDir, alphaDir, interpolatedFrameDir, interpolatedAlphaDir, mergedDir} {
		err = os.Mkdir(childDir, 0600)
		if err != nil {
			return 0, fmt.Errorf("error creating temporary subdirectory:\n  %s", err)
		}
	}

	// Get information about the source animation

	output, err := exec.Command("magick", "identify", "-format", "%n %T ", source).Output()
	if err != nil {
		return 0, fmt.Errorf("error getting number of frames in source:\n  %s", err)
	}

	var frameCount uint64
	var frameLength uint64
	_, err = fmt.Sscan(string(output), &frameCount, &frameLength)
	if err != nil {
		return 0, fmt.Errorf("error reading number of frames in source:\n  %s", err)
	}

	if frameCount <= 1 {
		return 0, fmt.Errorf("error reading source frames:\n  Found 1 or fewer frames in source; nothing to interpolate.")
	}

	inputPaddingSpecifier := fmt.Sprintf("%%0%dd.png", len(strconv.FormatUint(frameCount, 10))) // E.g. %02d.png

	// Extract frames and frame alpha

	errChannel := make(chan error)

	go func(result chan error) {
		localErr := exec.Command("magick", "convert", source, "-background", background, "-coalesce", "-alpha", "Background", "-alpha", "Off", "-strip", "-define", "png:color-type=2", path.Join(frameDir, inputPaddingSpecifier)).Run()
		if localErr != nil {
			result <- fmt.Errorf("error extracting frames from source:\n  %s", localErr)
			return
		}
		result <- nil
	}(errChannel)

	go func(result chan error) {
		localErr := exec.Command("magick", "convert", source, "-coalesce", "-alpha", "Extract", "-strip", "-define", "png:color-type=0", path.Join(alphaDir, inputPaddingSpecifier)).Run()
		if localErr != nil {
			result <- fmt.Errorf("error extracting alpha from source frames:\n  %s", localErr)
			return
		}
		result <- nil
	}(errChannel)

	if err = coalesce(2, errChannel); err != nil {
		return 0, err
	}

	// Copy the first frame to the end, for smoother looping

	for _, childDir := range []string{frameDir, alphaDir} {
		firstFrame := path.Join(childDir, fmt.Sprintf(inputPaddingSpecifier, 0))
		lastFrame := path.Join(childDir, fmt.Sprintf(inputPaddingSpecifier, frameCount))
		err = os.Link(firstFrame, lastFrame)
		if err != nil {
			// Maybe hardlinking just isn't supported
			_, err = copyFile(firstFrame, lastFrame)
			if err != nil {
				return 0, fmt.Errorf("error duplicating first frame:\n  %s", err)
			}
		}
	}

	// Perform interpolation

	// Numbering from 1, and including the first half of the interpolated duplicate frame pair
	finalFrameCount := frameCount*2 + 1
	outputPaddingSpecifier := fmt.Sprintf("%%0%dd.png", len(strconv.FormatUint(finalFrameCount, 10)))

	go func(result chan error) {
		localErr := exec.Command("rife", "-m", "rife-v4.6", "-i", frameDir, "-o", interpolatedFrameDir, "-x", "-z", "-f", outputPaddingSpecifier).Run()
		if localErr != nil {
			result <- fmt.Errorf("error interpolating frames:\n  %s", localErr)
			return
		}
		result <- nil
	}(errChannel)

	go func(result chan error) {
		localErr := exec.Command("rife", "-m", "rife-v4.6", "-i", alphaDir, "-o", interpolatedAlphaDir, "-x", "-z", "-f", outputPaddingSpecifier).Run()
		if localErr != nil {
			result <- fmt.Errorf("error interpolating alpha:\n  %s", localErr)
			return
		}
		result <- nil
	}(errChannel)

	if err = coalesce(2, errChannel); err != nil {
		return 0, err
	}

	// Merge alpha channel with opaque frames

	for frame := uint64(1); frame <= finalFrameCount; frame++ {
		// RIFE output is numbered starting from 1
		go func(i uint64, result chan error) {
			frameName := fmt.Sprintf(outputPaddingSpecifier, i)
			localErr := exec.Command(
				"magick", path.Join(interpolatedFrameDir, frameName), path.Join(interpolatedAlphaDir, frameName),
				"-alpha", "Off", "-compose", "CopyOpacity", "-composite", path.Join(mergedDir, frameName),
			).Run()
			if localErr != nil {
				result <- fmt.Errorf("error applying transparency to frames:\n  %s", localErr)
				return
			}
			result <- nil
		}(frame, errChannel)
	}

	if err = coalesce(finalFrameCount, errChannel); err != nil {
		return 0, err
	}

	close(errChannel)

	// Assemble into an APNG

	var framerateNumerator, framerateDenominator string
	if frameLength > 0 {
		// GIF frame lengths are in multiples of 1/100 of a second,
		// so for (roughly) twice as many frames, double the denominator to keep the duration the same.
		framerateNumerator = strconv.FormatUint(frameLength, 10)
		framerateDenominator = "200"
	} else {
		// Default to 10 FPS if there is no frame length.
		framerateNumerator = "1"
		framerateDenominator = "10"
	}
	err = exec.Command("apngasm64", dest, path.Join(mergedDir, "*.png"), "-i30", framerateNumerator, framerateDenominator).Run()
	if err != nil {
		return 0, fmt.Errorf("error assembling APNG:\n  %s", err)
	}

	return frameCount, nil
}

func coalesce(count uint64, errChannel chan error) error {
	var err error
	for i := uint64(0); i < count; i++ {
		if procErr := <-errChannel; procErr != nil && err == nil {
			// Save only the first error, but wait for all channels to report back
			err = procErr
		}
	}
	return err
}

func copyFile(src, dst string) (int64, error) {
	// From https://opensource.com/article/18/6/copying-files-go
	sourceFileStat, err := os.Stat(src)
	if err != nil {
		return 0, err
	}

	if !sourceFileStat.Mode().IsRegular() {
		return 0, fmt.Errorf("%s is not a regular file", src)
	}

	source, err := os.Open(src)
	if err != nil {
		return 0, err
	}
	defer func(source *os.File) { _ = source.Close() }(source)

	destination, err := os.Create(dst)
	if err != nil {
		return 0, err
	}
	defer func(destination *os.File) { _ = destination.Close() }(destination)
	nBytes, err := io.Copy(destination, source)
	return nBytes, err
}
