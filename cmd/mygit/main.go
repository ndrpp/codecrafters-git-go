package main

import (
	"bytes"
	"compress/zlib"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Usage: your_git.sh <command> <arg1> <arg2> ...
func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "usage: mygit <commad> [<args>...]\n")
		os.Exit(1)
	}

	switch command := os.Args[1]; command {
	case "init":
		dirs := []string{".git", ".git/objects", ".git/refs"}
		for _, dir := range dirs {
			if err := os.Mkdir(dir, 0755); err != nil {
				fmt.Fprintf(os.Stderr, "Error creating directory: %s\n", err)
			}
		}

		headContent := []byte("ref: refs/heads/main\n")
		if err := os.WriteFile(".git/HEAD", headContent, 0644); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing file: %s\n", err)
		}

		fmt.Println("Successfully initialized git directory.")

	case "cat-file":
		if len(os.Args) < 4 {
			fmt.Fprintf(os.Stderr, "usage: mygit cat-file <flag> [<args>...]\n")
			os.Exit(1)
		}
		catFile(os.Args[3], os.Args[2])

	case "hash-object":
		if len(os.Args) < 4 {
			fmt.Fprintf(os.Stderr, "usage: mygit hash-object <flag> [<args>...]\n")
			os.Exit(1)
		}
		hashObject(os.Args[2], os.Args[3])

	case "ls-tree":
		if len(os.Args) == 4 {
			listTree(os.Args[2], os.Args[3])
		} else {
			listTree("unflagged", os.Args[2])
		}

	case "write-tree":
		treeHash, err := writeTree(".")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to write tree: %s\n", err)
		}
		fmt.Fprintf(os.Stdout, hex.EncodeToString(treeHash[:]))

	default:
		fmt.Fprintf(os.Stderr, "Unsupported command.")
		os.Exit(1)
	}
}

type TreeEntry struct {
	Mode string
	Name string
	Hash [20]byte
}

func writeTree(dir string) ([20]byte, error) {
	var entries []TreeEntry

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if path == dir {
			return nil
		}

		if info.Name() == ".git" {
			return filepath.SkipDir
		}

		if !info.IsDir() {
			file, err := os.Open(path)
			if err != nil {
				return err
			}
			reader := io.Reader(file)
			content := make([]byte, 1024)
			num, err := reader.Read(content)
			content = content[:num]
			hash := sha1.Sum(content)

			mode := "100644"
			if info.Mode()&0111 != 0 {
				mode = "100755"
			}

			name := filepath.Base(path)
			entries = append(entries, TreeEntry{Mode: mode, Name: name, Hash: hash})
		} else {
			mode := "40000"
			name := filepath.Base(path)
			hash, err := writeTree(name)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Failed to parse inner tree")
				os.Exit(1)
			}

			entries = append(entries, TreeEntry{Mode: mode, Name: name, Hash: hash})
			return filepath.SkipDir
		}

		return nil
	})

	if err != nil {
		return [20]byte{}, err
	}

	var treeContent bytes.Buffer
	for _, entry := range entries {
		fmt.Fprintf(&treeContent, "%s %s\x00", entry.Mode, entry.Name)
		treeContent.Write(entry.Hash[:])
	}

	treeHash := sha1.Sum(treeContent.Bytes())

	treeObject := fmt.Sprintf("tree %d\x00%s", treeContent.Len(), treeContent.Bytes())

	var buff bytes.Buffer
	z := zlib.NewWriter(&buff)
	z.Write([]byte(treeObject))
	z.Close()

	hashStr := hex.EncodeToString(treeHash[:])
	objectPath := fmt.Sprintf(".git/objects/%s/%s", hashStr[:2], hashStr[2:])
	os.MkdirAll(filepath.Dir(objectPath), 0755)
	os.WriteFile(objectPath, buff.Bytes(), 0644)

	return treeHash, nil
}

func parseTree(sha string) ([][]string, int) {
	p, err := zlibDecompress(fmt.Sprintf(".git/objects/%s/%s", sha[0:2], sha[2:]))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to decompress file: %s\n", err)
	}

	var result [][]string
	arr := strings.SplitN(string(p), "\x00", 2)
	entries := strings.Split(arr[1], "\x00")
	for i := range entries {
		if i == 0 {
			result = append(result, strings.Split(entries[i], " "))
		} else if i == len(entries)-1 {
			result[len(result)-1] = append(result[i-1], hex.EncodeToString([]byte(entries[i])))
		} else {
			if entries[i] != "" {
				hash := entries[i][:20]
				result[i-1] = append(result[i-1], hex.EncodeToString([]byte(hash)))
				result = append(result, []string{entries[i][20:26], entries[i][26:]})
			}
		}
	}

	return result, len(entries)
}

func listTree(flag, sha string) {
	switch flag {
	case "unflagged":
		result, length := parseTree(sha)
		for i := 0; i < length-1; i++ {
			fmt.Println(fmt.Sprintf("%s %s %s", strings.TrimSpace(result[i][0]), strings.TrimSpace(result[i][2]), strings.TrimSpace(result[i][1])))
		}

	case "--name-only":
		result, length := parseTree(sha)
		for i := 0; i < length-1; i++ {
			fmt.Println(fmt.Sprintf("%s", strings.TrimSpace(result[i][1])))
		}

	default:
		fmt.Fprintf(os.Stderr, "Unsupported command.")
		os.Exit(1)
	}
}

func hashObject(flag, filename string) {
	switch flag {
	case "-w":
		createBlobObject(filename)

	default:
		fmt.Fprintf(os.Stderr, "Unsupported command.")
		os.Exit(1)
	}
}

func createBlobObject(filename string) string {
	file, err := os.Open(fmt.Sprintf("%s", filename))
	if err != nil {
		fmt.Fprintf(os.Stderr, "File does not exist: %s\n", err)
		os.Exit(1)
	}
	reader := io.Reader(file)
	content := make([]byte, 1024)
	num, err := reader.Read(content)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to read content of file: %s\n", err)
		os.Exit(1)
	}
	content = content[0:num]
	content = []byte(fmt.Sprintf("blob %d\x00", len(string(content))) + string(content))

	hasher := sha1.New()
	_, err = hasher.Write(content)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to sha1 hash the file content: %s\n", err)
		os.Exit(1)
	}
	sha := hasher.Sum(nil)
	result := hex.EncodeToString(sha)

	if err := os.Mkdir(fmt.Sprintf(".git/objects/%s", result[0:2]), 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating directory: %s\n", err)
		os.Exit(1)
	}

	var b bytes.Buffer
	w := zlib.NewWriter(&b)
	w.Write(content)
	w.Close()
	compressed := b.Bytes()
	if err := os.WriteFile(fmt.Sprintf(".git/objects/%s/%s", result[0:2], result[2:]), compressed, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing file: %s\n", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stdout, result)

	return result
}

func catFile(sha, flag string) {
	switch flag {
	case "-p":
		p, err := zlibDecompress(fmt.Sprintf(".git/objects/%s/%s", sha[0:2], sha[2:]))
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to decompress file: %s\n", err)
		}
		fmt.Print(strings.Split(string(p), "\x00")[1])

	default:
		fmt.Fprintf(os.Stderr, "Unsupported command.")
		os.Exit(1)
	}
}

func zlibDecompress(filename string) ([]byte, error) {
	file, err := os.Open(filename)
	if err != nil {
		fmt.Fprintf(os.Stderr, "File does not exist: %s\n", err)
		return nil, err
	}

	z, err := zlib.NewReader(io.Reader(file))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create zlib reader: %s\n", err)
		z.Close()
		return nil, err
	}

	p, err := io.ReadAll(z)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to read compressed data: %s\n", err)
		z.Close()
		return nil, err
	}

	z.Close()
	return p, nil
}
