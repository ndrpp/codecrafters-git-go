package main

import (
	"bytes"
	"compress/zlib"
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
		if len(os.Args) < 4 || os.Args[2] != "-p" {
			fmt.Fprintf(os.Stderr, "usage: mygit cat-file -p [<args>...]\n")
			os.Exit(1)
		}

		hash := os.Args[3]
		content, err := os.ReadFile(fmt.Sprintf(".git/objects/%s/%s", hash[0:2], hash[2:len(hash)-1]))
		if err != nil {
			fmt.Fprintf(os.Stderr, "File does not exist: %s\n", err)
			os.Exit(1)
		}
		b := bytes.NewReader(content)
		z, err := zlib.NewReader(b)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to create zlib reader: %s\n", err)
			os.Exit(1)
		}
		defer z.Close()
		p, err := io.ReadAll(z)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to read compressed data: %s\n", err)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stdout, strings.Split(string(p), "\\0")[1])

	default:
		fmt.Fprintf(os.Stderr, "Unsupported command.")
		os.Exit(1)
	}
}
