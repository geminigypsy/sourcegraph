package main

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"github.com/bits-and-blooms/bloom/v3"
	"github.com/cockroachdb/errors"
	"github.com/go-enry/go-enry/v2"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var (
	Yellow = color("\033[1;33m%s\033[0m")
)

const (
	estimate         = 0.01
	maxFileSize      = 1 << 20 // 1_048_576
	bloomSizePadding = 10
)

type RepoIndex struct {
	Dir   string
	Blobs []BlobIndex
}
type BlobIndex struct {
	Filter *bloom.BloomFilter
	Path   string
}

func onGrams(textBytes []byte, onBytes func(b []byte)) {
	for i, _ := range textBytes {
		onBytes(textBytes[i : i+1]) // unigram
		if i > 0 {
			onBytes(textBytes[i-1 : i+1]) // bigram
		}
		if i > 1 {
			onBytes(textBytes[i-2 : i+1]) // trigram
		}
		if i > 2 {
			onBytes(textBytes[i-3 : i+1]) // quadgram
		}
		if i > 3 {
			onBytes(textBytes[i-4 : i+1]) // pentagram
		}
		if i > 4 {
			onBytes(textBytes[i-3 : i+1]) // hexagram
		}
	}
}

func collectGrams(query string) [][]byte {
	var result [][]byte
	onGrams([]byte(query), func(b []byte) {
		result = append(result, b)
	})
	return result
}

func (r *RepoIndex) SerializeToFile(cacheDir string) (err error) {
	_ = os.Remove(cacheDir)
	err = os.MkdirAll(filepath.Dir(cacheDir), 0755)
	if err != nil {
		return err
	}
	cacheOut, err := os.Create(cacheDir)
	if err != nil {
		return err
	}
	defer func() {
		closeErr := cacheOut.Close()
		if err != nil {
			err = closeErr
		}
	}()
	err = r.Serialize(cacheOut)
	return
}

func (r *RepoIndex) Serialize(w io.Writer) error {
	return gob.NewEncoder(w).Encode(r)
}

func DeserializeRepoIndex(reader io.Reader) (*RepoIndex, error) {
	var r *RepoIndex
	err := gob.NewDecoder(reader).Decode(r)
	if err != nil {
		return nil, err
	}
	return r, nil
}

func NewRepoIndex(dir string) (*RepoIndex, error) {
	var branch bytes.Buffer
	branchCmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	branchCmd.Dir = dir
	branchCmd.Stdout = &branch
	err := branchCmd.Run()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to infer the default branch")
	}
	cmd := exec.Command(
		"git",
		"ls-files",
		"-z",
		"--with-tree",
		strings.Trim(branch.String(), "\n"),
	)
	cmd.Dir = dir
	var out bytes.Buffer
	cmd.Stdout = &out
	err = cmd.Run()

	if err != nil {
		return nil, err
	}
	stdout := string(out.Bytes())
	NUL := string([]byte{0})
	lines := strings.Split(stdout, NUL)
	indexes := make([]BlobIndex, len(lines))
	for i, line := range lines {
		if i%100 == 0 {
			fmt.Println(i)
		}
		abspath := path.Join(dir, line)
		textBytes, err := os.ReadFile(abspath)
		if err != nil {
			continue
		}
		if len(textBytes) > maxFileSize {
			continue
		}
		bloomSize := uint(len(textBytes) * bloomSizePadding)
		filter := bloom.NewWithEstimates(bloomSize, estimate)
		if enry.IsBinary(textBytes) {
			continue
		}
		onGrams(textBytes, func(b []byte) {
			filter.Add(b)
		})
		sizeRatio := float64(filter.ApproximatedSize()) / float64(bloomSize)
		if sizeRatio > 0.5 {
			fmt.Printf("%v %v %v\n", sizeRatio, filter.ApproximatedSize(), bloomSize)
		}
		indexes = append(
			indexes,
			BlobIndex{
				Path:   line,
				Filter: filter,
			},
		)
	}
	return &RepoIndex{Dir: dir, Blobs: indexes}, nil
}

func (r *RepoIndex) Grep(query string) {
	start := time.Now()
	matchingPaths := r.PathsMatchingQuery(query)
	falsePositive := 0
	truePositive := 0
	for matchingPath := range matchingPaths {
		hasMatch := false
		textBytes, err := os.ReadFile(filepath.Join(r.Dir, matchingPath))
		if err != nil {
			continue
		}
		text := string(textBytes)
		start := 0
		end := strings.Index(text[start:], "\n")
		for lineNumber, line := range strings.Split(text, "\n") {
			columnNumber := strings.Index(line, query)
			if columnNumber >= 0 {
				hasMatch = true
				prefix := line[0:columnNumber]
				suffix := line[columnNumber+len(query):]
				fmt.Printf(
					"%v:%v:%v %v%v%v\n",
					matchingPath,
					lineNumber,
					columnNumber,
					prefix,
					Yellow(query),
					suffix,
				)
			}
			start = end + 1
			end = strings.Index(text[end+1:], "\n")
		}

		if hasMatch {
			truePositive++
		} else {
			//fmt.Println(matchingPath)
			falsePositive++
		}
	}
	end := time.Now()
	elapsed := (end.UnixNano() - start.UnixNano()) / int64(time.Millisecond)
	falsePositiveRatio := float64(falsePositive) / float64(truePositive+falsePositive)
	fmt.Printf("query '%v' time %vms fpr %v\n", query, elapsed, falsePositiveRatio)
}

func color(colorString string) func(...interface{}) string {
	sprint := func(args ...interface{}) string {
		return fmt.Sprintf(colorString,
			fmt.Sprint(args...))
	}
	return sprint
}

func (r *RepoIndex) PathsMatchingQuery(query string) chan string {
	grams := collectGrams(query)
	res := make(chan string, len(r.Blobs))
	batchSize := 5_000
	var wg sync.WaitGroup
	for i := 0; i < len(r.Blobs); i += batchSize {
		j := i + batchSize
		if j > len(r.Blobs) {
			j = len(r.Blobs)
		}
		batch := r.Blobs[i:j]
		wg.Add(1)
		go func() {
			defer wg.Done()
			for _, index := range batch {
				if index.Filter == nil {
					continue
				}
				isMatch := true
				for _, gram := range grams {
					if !index.Filter.Test(gram) {
						isMatch = false
						break
					}
				}
				if isMatch {
					res <- index.Path
				}
			}
		}()
	}
	wg.Wait()
	close(res)
	return res
}
