package engine

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/juanrubio/outfitter/internal/catalog"
	"github.com/juanrubio/outfitter/internal/domain"
)

type runnerCall struct {
	Name string
	Args []string
}

type fakeRunner struct {
	mu      sync.Mutex
	calls   []runnerCall
	respond func(ctx context.Context, name string, args ...string) ([]byte, error)
}

func (f *fakeRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	f.mu.Lock()
	f.calls = append(f.calls, runnerCall{Name: name, Args: append([]string(nil), args...)})
	respond := f.respond
	f.mu.Unlock()
	if respond != nil {
		return respond(ctx, name, args...)
	}
	return nil, nil
}

func (f *fakeRunner) callsSnapshot() []runnerCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]runnerCall(nil), f.calls...)
}

func (f *fakeRunner) called(name string, args ...string) bool {
	for _, call := range f.callsSnapshot() {
		if call.Name != name || len(call.Args) != len(args) {
			continue
		}
		match := true
		for index, arg := range args {
			if call.Args[index] != arg {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

func writeTestFile(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir for %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatalf("chmod %s: %v", path, err)
	}
}

type tarFile struct {
	Name    string
	Content string
	Mode    int64
}

func makeTarGz(t *testing.T, files []tarFile) []byte {
	t.Helper()
	var buffer bytes.Buffer
	gzipWriter := gzip.NewWriter(&buffer)
	tarWriter := tar.NewWriter(gzipWriter)

	for _, file := range files {
		mode := file.Mode
		if mode == 0 {
			mode = 0o755
		}
		header := &tar.Header{
			Name:     file.Name,
			Mode:     mode,
			Size:     int64(len(file.Content)),
			Typeflag: tar.TypeReg,
			ModTime:  time.Unix(0, 0),
		}
		if err := tarWriter.WriteHeader(header); err != nil {
			t.Fatalf("write tar header %s: %v", file.Name, err)
		}
		if _, err := tarWriter.Write([]byte(file.Content)); err != nil {
			t.Fatalf("write tar body %s: %v", file.Name, err)
		}
	}

	if err := tarWriter.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatalf("close gzip: %v", err)
	}
	return buffer.Bytes()
}

func testCatalog(tools ...domain.Tool) *catalog.Catalog {
	settings := domain.DefaultSettings()
	settings.GitHubTokenFromGH = false
	return &catalog.Catalog{
		Settings: settings,
		Tools:    tools,
	}
}

func testTool(id, binary string, backend domain.BackendType, pkg string) domain.Tool {
	return domain.Tool{
		ID:           id,
		Name:         id,
		Binary:       binary,
		Backend:      backend,
		Package:      pkg,
		VersionArgs:  []string{"--version"},
		VersionRegex: "",
	}
}
