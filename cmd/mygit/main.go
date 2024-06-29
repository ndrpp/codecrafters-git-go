package main

import (
	"bytes"
	"compress/zlib"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"os"
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
		arr := make([]string, 0, 10)
		writeTree(".", arr)

	default:
		fmt.Fprintf(os.Stderr, "Unsupported command.")
		os.Exit(1)
	}
}

func writeTree(dir string, arr []string) {
	files, err := os.ReadDir(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open directory: %s", err)
		os.Exit(1)
	}

	for i := 0; i < len(files); i++ {
		if !files[i].IsDir() {
			sha := createBlobObject(files[i].Name())

			arr = append(arr, fmt.Sprintf("blob %s\x00%s", files[i].Name(), sha))
		}
	}

	fmt.Println("arr: ", arr)
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
