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

	var fileEvents <-chan string
	var stopFunc func()
	if *watch {
		fileEvents, stopFunc = watchDirEvents(*inputDir)
		defer stopFunc()
	}

	for {
		var filesToProcess []os.FileInfo
		var changedFile string

		if *watch {
			// Wait for a file event
			changedFile = <-fileEvents
			if changedFile == "" {
				continue
			}
			fullPath := filepath.Join(*inputDir, changedFile)
			fi, err := os.Stat(fullPath)
			if err != nil || fi.IsDir() {
				continue // Ignore if not found or is a directory
			}
			filesToProcess = []os.FileInfo{fi}
		} else {
			files, err := ioutil.ReadDir(*inputDir)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Failed to read %s: %v\n", *inputDir, err)
				os.Exit(1)
			}
			filesToProcess = files
		}

		// Read mapping from outputJSON if it exists
		mapping := make(map[string]string)
		if data, err := ioutil.ReadFile(*outputJSON); err == nil && len(data) > 0 {
			_ = json.Unmarshal(data, &mapping)
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

			fmt.Fprintf(os.Stderr, "Updated: %s\n", hashFilename)
		}

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

// Efficiently watches a directory for create/modify file events and sends filenames via channel.
// Returns a channel and a stop function.
func watchDirEvents(dir string) (<-chan string, func()) {
	events := make(chan string)
	fd, err := syscall.InotifyInit()
	if err != nil {
		fmt.Fprintf(os.Stderr, "inotify init error: %v\n", err)
		os.Exit(1)
	}
	wd, err := syscall.InotifyAddWatch(fd, dir, syscall.IN_CREATE|syscall.IN_MODIFY)
	if err != nil {
		fmt.Fprintf(os.Stderr, "inotify addwatch error: %v\n", err)
		os.Exit(1)
	}

	stop := make(chan struct{})
	go func() {
		defer func() {
			syscall.InotifyRmWatch(fd, uint32(wd))
			syscall.Close(fd)
			close(events)
		}()
		var buf [4096]byte
		for {
			select {
			case <-stop:
				return
			default:
			}
			n, err := syscall.Read(fd, buf[:])
			if err != nil {
				fmt.Fprintf(os.Stderr, "inotify read error: %v\n", err)
				return
			}
			var offset uint32
			for offset+syscall.SizeofInotifyEvent <= uint32(n) {
				raw := (*syscall.InotifyEvent)(unsafe.Pointer(&buf[offset]))
				nameBytes := buf[offset+syscall.SizeofInotifyEvent : offset+syscall.SizeofInotifyEvent+raw.Len]
				// Remove trailing null bytes from name
				name := strings.TrimRight(string(nameBytes), "\x00")
				if raw.Mask&(syscall.IN_CREATE|syscall.IN_MODIFY) != 0 && name != "" {
					events <- name
				}
				offset += syscall.SizeofInotifyEvent + raw.Len
			}
		}
	}()
	stopFunc := func() { close(stop) }
	return events, stopFunc
}
