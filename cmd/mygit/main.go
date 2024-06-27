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

	default:
		fmt.Fprintf(os.Stderr, "Unsupported command.")
		os.Exit(1)
	}
}

func hashObject(flag, filename string) {
	switch flag {
	case "-w":
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

	default:
		fmt.Fprintf(os.Stderr, "Unsupported command.")
		os.Exit(1)
	}
}

func catFile(sha, flag string) {
	switch flag {
	case "-p":
		file, err := os.Open(fmt.Sprintf(".git/objects/%s/%s", sha[0:2], sha[2:]))
		if err != nil {
			fmt.Fprintf(os.Stderr, "File does not exist: %s\n", err)
			os.Exit(1)
		}

		b := io.Reader(file)
		z, err := zlib.NewReader(b)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to create zlib reader: %s\n", err)
			os.Exit(1)
		}

		p, err := io.ReadAll(z)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to read compressed data: %s\n", err)
			os.Exit(1)
		}
		fmt.Print(strings.Split(string(p), "\x00")[1])
		z.Close()

	default:
		fmt.Fprintf(os.Stderr, "Unsupported command.")
		os.Exit(1)
	}
}
