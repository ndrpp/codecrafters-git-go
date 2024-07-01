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
	"sort"
	"strings"
)

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
		cur, err := os.Getwd()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to open working directory.")
			os.Exit(1)
		}

		sha, c := writeTree(cur)
		treeHash := hex.EncodeToString(sha)

		if err := os.Mkdir(fmt.Sprintf(".git/objects/%s", treeHash[0:2]), 0755); err != nil {
			fmt.Fprintf(os.Stderr, "Error creating directory: %s\n", err)
			os.Exit(1)
		}

		var b bytes.Buffer
		w := zlib.NewWriter(&b)
		w.Write(c)
		w.Close()
		compressed := b.Bytes()
		if err := os.WriteFile(fmt.Sprintf(".git/objects/%s/%s", treeHash[0:2], treeHash[2:]), compressed, 0644); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing file: %s\n", err)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stdout, treeHash)

	case "commit-tree":
		hash := commitTree(os.Args[2], os.Args[4], os.Args[6])

		fmt.Fprintf(os.Stdout, hash)

	default:
		fmt.Fprintf(os.Stderr, "Unsupported command.")
		os.Exit(1)
	}
}

func commitTree(treeSha, commitSha, message string) string {
	str := fmt.Sprintf("tree %s\nparent %s\nauthor ndrpp <email@gmail.com> 1719851420 +0300\ncommiter ndrpp <email@gmail.com> 1719851420 +0300\n\n%s\n", treeSha, commitSha, message)
	content := []byte(fmt.Sprintf("commit %d\x00", len(str)) + str)

	hasher := sha1.New()
	_, err := hasher.Write(content)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to sha1 hash the commit: %s\n", err)
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

	return result
}

func writeTree(dir string) ([]byte, []byte) {
	fileInfos, err := os.ReadDir(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to read directory.")
		os.Exit(1)
	}

	type entry struct {
		fileName string
		b        []byte
	}
	var entries []entry
	contentSize := 0

	for _, fileInfo := range fileInfos {
		if fileInfo.Name() == ".git" {
			continue
		}

		if !fileInfo.IsDir() {
			f, err := os.Open(filepath.Join(dir, fileInfo.Name()))
			if err != nil {
				fmt.Fprintf(os.Stderr, "Failed to open file: %s\n", err)
				os.Exit(1)
			}
			b, err := io.ReadAll(f)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Failed to read file: %s\n", err)
				os.Exit(1)
			}

			s := fmt.Sprintf("blob %d\u0000%s", len(b), string(b))
			sha1 := sha1.New()
			io.WriteString(sha1, s)
			s = fmt.Sprintf("100644 %s\u0000", fileInfo.Name())
			b = append([]byte(s), sha1.Sum(nil)...)
			entries = append(entries, entry{fileInfo.Name(), b})
			contentSize += len(b)
		} else {
			b, _ := writeTree(filepath.Join(dir, fileInfo.Name()))
			s := fmt.Sprintf("40000 %s\u0000", fileInfo.Name())
			b2 := append([]byte(s), b...)
			entries = append(entries, entry{fileInfo.Name(), b2})
			contentSize += len(b2)
		}
	}

	sort.Slice(entries, func(i, j int) bool { return entries[i].fileName < entries[j].fileName })
	s := fmt.Sprintf("tree %d\u0000", contentSize)
	b := []byte(s)

	for _, entry := range entries {
		b = append(b, entry.b...)
	}
	sha1 := sha1.New()
	io.WriteString(sha1, string(b))
	return sha1.Sum(nil), b
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
			if len(entries[i]) >= 20 {
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
