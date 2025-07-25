package main

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

func main() {
	inputDir := flag.String("input-dir", "", "Input directory")
	outputJSON := flag.String("output-json", "", "Output JSON file")
	outputDir := flag.String("output-dir", "", "Output directory")
	watch := flag.Bool("watch", false, "Watch mode")

	flag.Parse()

	if *inputDir == "" || *outputJSON == "" || *outputDir == "" {
		fmt.Fprintf(os.Stderr, "Usage: %s --input-dir <input_dir> --output-json <output_json> --output-dir <output_dir> [--watch]\n", os.Args[0])
		os.Exit(1)
	}

	for {
		if *watch {
			waitForCreateOrModify(*inputDir)
		}

		_ = ioutil.WriteFile(*outputJSON, []byte("{}"), 0644)

		files, err := ioutil.ReadDir(*inputDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to read %s: %v\n", *inputDir, err)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "Found %d files in %s\n", len(files), *inputDir)

		mapping := make(map[string]string)
		for _, fi := range files {
			if fi.IsDir() {
				continue
			}
			filename := fi.Name()
			filenameNoExt := strings.TrimSuffix(filename, filepath.Ext(filename))
			ext := filepath.Ext(filename)
			ext = strings.TrimPrefix(ext, ".")

			fullPath := filepath.Join(*inputDir, filename)
			hash, err := md5File(fullPath)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Failed to hash %s: %v\n", filename, err)
				continue
			}
			hashFilename := fmt.Sprintf("%s.%s.%s", filenameNoExt, hash, ext)
			mapping[filename] = hashFilename

			err = copyFile(fullPath, filepath.Join(*outputDir, hashFilename))
			if err != nil {
				fmt.Fprintf(os.Stderr, "Failed to copy %s: %v\n", filename, err)
				continue
			}

			fmt.Fprintf(os.Stderr, "Copied %s to %s\n", filename, hashFilename)
		}

		fmt.Fprintf(os.Stderr, "Generated mapping: %v\n", mapping)

		outData, _ := json.MarshalIndent(mapping, "", "  ")
		_ = ioutil.WriteFile(*outputJSON, outData, 0644)

		if !*watch {
			break
		}
	}
}

func md5File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := md5.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() {
		cerr := out.Close()
		if err == nil {
			err = cerr
		}
	}()
	if _, err = io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}

func waitForCreateOrModify(dir string) {
	fd, err := syscall.InotifyInit()
	if err != nil {
		fmt.Fprintf(os.Stderr, "inotify init error: %v\n", err)
		os.Exit(1)
	}
	defer syscall.Close(fd)

	wd, err := syscall.InotifyAddWatch(fd, dir, syscall.IN_CREATE|syscall.IN_MODIFY)
	if err != nil {
		fmt.Fprintf(os.Stderr, "inotify addwatch error: %v\n", err)
		os.Exit(1)
	}
	defer syscall.InotifyRmWatch(fd, uint32(wd))

	var buf [4096]byte
	_, err = syscall.Read(fd, buf[:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "inotify read error: %v\n", err)
	}
}
