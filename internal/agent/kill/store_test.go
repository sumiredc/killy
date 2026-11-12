package kill

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sumiredc/killy/internal/protocol"
)

const (
	idA = "0f8fad5b-d9cb-469f-a165-70867728950e"
	idB = "7c9e6679-7425-40de-944b-e07fc1f90ae7"
	idC = "16fd2706-8baf-433b-82eb-8c7fada847da"
)

var (
	killedTwo  = protocol.Outcome{Outcome: protocol.Killed, Reaped: 2}
	failedPerm = protocol.Outcome{Outcome: protocol.SignalFailed, Reason: "EPERM"}
	at         = time.Date(2026, 11, 13, 0, 0, 0, 0, time.UTC)
)

func storePath(t *testing.T) string {
	return filepath.Join(t.TempDir(), "outcomes.jsonl")
}

func openStore(t *testing.T, path string) *FileStore {
	t.Helper()
	s, err := OpenFileStore(path)
	if err != nil {
		t.Fatalf("OpenFileStore: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func assertLookup(t *testing.T, s OutcomeStore, id string, want protocol.Outcome) {
	t.Helper()
	got, ok := s.Lookup(id)
	if !ok || got != want {
		t.Errorf("Lookup(%s) = %+v, %v; want %+v, true", id, got, ok, want)
	}
}

func assertNoRecord(t *testing.T, s OutcomeStore, id string) {
	t.Helper()
	if got, ok := s.Lookup(id); ok {
		t.Errorf("Lookup(%s) = %+v, want no record", id, got)
	}
}

func TestFileStoreStartsEmptyWhenTheFileDoesNotExist(t *testing.T) {
	s := openStore(t, storePath(t))
	assertNoRecord(t, s, idA)
}

func TestFileStoreReturnsWhatWasSaved(t *testing.T) {
	s := openStore(t, storePath(t))
	if err := s.Save(idA, killedTwo, at); err != nil {
		t.Fatal(err)
	}
	assertLookup(t, s, idA, killedTwo)
	assertNoRecord(t, s, idB)
}

func TestFileStoreCreatesTheFileWithMode0600(t *testing.T) {
	path := storePath(t)
	s := openStore(t, path)
	if err := s.Save(idA, killedTwo, at); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("mode = %o, want 600", perm)
	}
}

func TestFileStoreKeepsRecordsAcrossReopening(t *testing.T) {
	path := storePath(t)
	s := openStore(t, path)
	if err := s.Save(idA, killedTwo, at); err != nil {
		t.Fatal(err)
	}
	if err := s.Save(idB, failedPerm, at); err != nil {
		t.Fatal(err)
	}
	_ = s.Close()

	reopened := openStore(t, path)
	assertLookup(t, reopened, idA, killedTwo)
	assertLookup(t, reopened, idB, failedPerm)
}

// writeTruncated leaves a file whose last record was cut off mid-write, as after a power loss.
func writeTruncated(t *testing.T, path string) {
	t.Helper()
	data := `{"id":"` + idA + `","outcome":"killed","reaped":2,"at":"2026-11-13T00:00:00Z"}` + "\n" +
		`{"id":"` + idB + `","outcome":"kil`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestFileStoreReadsTheRecordsBeforeATruncatedLastLine(t *testing.T) {
	path := storePath(t)
	writeTruncated(t, path)
	s := openStore(t, path)
	assertLookup(t, s, idA, killedTwo)
	assertNoRecord(t, s, idB)
}

func TestFileStoreKeepsANewRecordAfterATruncatedLastLineReadable(t *testing.T) {
	path := storePath(t)
	writeTruncated(t, path)
	s := openStore(t, path)
	if err := s.Save(idC, failedPerm, at); err != nil {
		t.Fatal(err)
	}
	_ = s.Close()

	reopened := openStore(t, path)
	assertLookup(t, reopened, idA, killedTwo)
	assertLookup(t, reopened, idC, failedPerm)
}

func TestFileStoreFailsWhenTheFileExistsButCannotBeRead(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads a file whatever its mode")
	}
	path := storePath(t)
	// Write-only: it could still be appended to, so only the read can catch it.
	if err := os.WriteFile(path, nil, 0o200); err != nil {
		t.Fatal(err)
	}
	if s, err := OpenFileStore(path); err == nil {
		_ = s.Close()
		t.Error("OpenFileStore succeeded on a file it cannot read")
	}
}

func TestFileStoreFailsWhenThePathIsADirectory(t *testing.T) {
	path := storePath(t)
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}
	if s, err := OpenFileStore(path); err == nil {
		_ = s.Close()
		t.Error("OpenFileStore succeeded on a path that is a directory")
	}
}

// failingFile is an appendFile whose writes always fail, as on a full disk.
type failingFile struct{ writes int }

func (f *failingFile) Write([]byte) (int, error) {
	f.writes++
	return 0, errors.New("no space left on device")
}
func (f *failingFile) Sync() error  { return nil }
func (f *failingFile) Close() error { return nil }

func TestFileStoreKeepsTheOutcomeInMemoryWhenTheWriteFails(t *testing.T) {
	s := openStore(t, storePath(t))
	file := &failingFile{}
	s.file = file
	if err := s.Save(idA, killedTwo, at); err == nil {
		t.Error("Save succeeded although the write failed")
	}
	assertLookup(t, s, idA, killedTwo)
	if file.writes != 1 {
		t.Errorf("tried to write %d times, want 1", file.writes)
	}
}
