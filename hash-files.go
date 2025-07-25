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
	"unsafe"
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
		var changedFile string

		if *watch {
			var err error
			changedFile, err = waitForCreateOrModify(*inputDir)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Failed to get changed file: %v\n", err)
				continue
			}
		}

		// Read mapping from outputJSON if it exists
		mapping := make(map[string]string)
		if data, err := ioutil.ReadFile(*outputJSON); err == nil && len(data) > 0 {
			_ = json.Unmarshal(data, &mapping)
		}

		var filesToProcess []os.FileInfo
		if *watch {
			// In watch mode, process only the changed file
			if changedFile == "" {
				continue // No file to process
			}
			fullPath := filepath.Join(*inputDir, changedFile)
			fi, err := os.Stat(fullPath)
			if err != nil || fi.IsDir() {
				continue // Ignore if not found or is a directory
			}
			filesToProcess = []os.FileInfo{fi}
		} else {
			// Otherwise, process all files in the directory
			files, err := ioutil.ReadDir(*inputDir)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Failed to read %s: %v\n", *inputDir, err)
				os.Exit(1)
			}
			filesToProcess = files
			fmt.Fprintf(os.Stderr, "Found %d files in %s\n", len(files), *inputDir)
		}

		for _, fi := range filesToProcess {
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

			// Check if the hashFilename matches the mapping
			if prev, exists := mapping[filename]; exists {
				if prev == hashFilename {
					continue
				}
			}

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

// Returns the changed filename (created/modified) in the watched directory, or "" if failed.
func waitForCreateOrModify(dir string) (string, error) {
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
	n, err := syscall.Read(fd, buf[:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "inotify read error: %v\n", err)
		return "", err
	}
	// inotify event struct: https://man7.org/linux/man-pages/man7/inotify.7.html
	// struct inotify_event {
	//   int      wd;
	//   uint32_t mask;
	//   uint32_t cookie;
	//   uint32_t len;
	//   char     name[];
	// }
	if n < 16 {
		return "", nil
	}
	nameLen := *(*uint32)(unsafe.Pointer(&buf[12]))
	if nameLen > 0 && int(16+nameLen) <= n {
		name := string(buf[16 : 16+nameLen])
		return strings.TrimRight(name, "\x00"), nil
	}
	return "", nil
}
