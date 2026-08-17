package sessions

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"iter"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/brentkeller/waid/internal/config"
	"github.com/brentkeller/waid/internal/events"
)

// CacheVersion is bumped whenever a cache written by an older waid can no longer be reused. The
// Node build wrote version 1, whose stat fields carry a millisecond mtime this build does not
// produce; bumping discards those files rather than trusting them.
const CacheVersion = 2

// maxLineBytes bounds a single transcript line. A record longer than this is a corrupt or foreign
// file rather than a session, and reading it would cost the memory the streaming reader exists to
// avoid.
const maxLineBytes = 16 << 20

// TranscriptFile is one transcript on disk, with the stats a sync compares against the cache.
type TranscriptFile struct {
	Path string
	// Dir is the name of the Claude Code project directory holding the file — a lossy encoding of
	// the cwd.
	Dir string
	// Id is the session id taken from the filename; only a fallback, since the transcript carries
	// its own.
	Id      string
	MtimeNs int64
	Size    int64
}

// FileStat is the transcript identity a cached session was parsed from.
type FileStat struct {
	Path    string `json:"path"`
	MtimeNs int64  `json:"mtimeNs"`
	Size    int64  `json:"size"`
}

// CachedSession is a Session plus the stat fields that let a sync skip re-reading an unchanged file.
type CachedSession struct {
	Session
	File FileStat `json:"_file"`
}

// Cache is cache/sessions.json as written. Disposable: deleting it costs one re-sync, nothing more.
type Cache struct {
	Version  int             `json:"version"`
	SyncedAt string          `json:"syncedAt"`
	Sessions []CachedSession `json:"sessions"`
}

// SyncStats counts what a sync did. Parsed plus reused can fall short of scanned when a transcript
// disappears mid-run.
type SyncStats struct {
	Scanned int `json:"scanned"`
	Parsed  int `json:"parsed"`
	Reused  int `json:"reused"`
	Removed int `json:"removed"`
}

// LineReader streams a transcript's lines. Line endings are left to the caller — FeedLine trims, so
// a CRLF file needs no special handling here.
type LineReader func(filePath string) iter.Seq2[string, error]

// SyncOptions tunes one sync.
type SyncOptions struct {
	// Full re-parses every transcript, ignoring the cached stats.
	Full bool
	// Now is the sync time recorded in the cache; the zero time means the wall clock.
	Now time.Time
	// ReadLines is the seam tests swap to observe or fake which transcripts are opened.
	ReadLines LineReader
}

// SyncResult is the cache a sync wrote, plus what it took to write it.
type SyncResult struct {
	Cache
	Stats SyncStats
}

// LoadedCache is a read of the cache. A missing, corrupt or foreign-version file reads as an empty
// one, with SyncedAt empty.
type LoadedCache struct {
	SyncedAt string
	Sessions []CachedSession
}

// ReadLines yields a file's lines without holding more than a buffer in memory, so a multi-megabyte
// transcript costs a buffer rather than its own size. An open or read failure is yielded as an error
// and ends the sequence.
func ReadLines(filePath string) iter.Seq2[string, error] {
	return func(yield func(string, error) bool) {
		file, err := os.Open(filePath)
		if err != nil {
			yield("", fmt.Errorf("opening %s: %w", filePath, err))
			return
		}
		defer file.Close()

		scanner := bufio.NewScanner(file)
		scanner.Buffer(nil, maxLineBytes)
		for scanner.Scan() {
			if !yield(scanner.Text(), nil) {
				return
			}
		}
		if err := scanner.Err(); err != nil {
			yield("", fmt.Errorf("reading %s: %w", filePath, err))
		}
	}
}

// DiscoverTranscripts lists every <claudeDir>/projects/<project-dir>/<session-id>.jsonl. A missing
// or unreadable Claude Code directory is not an error — plenty of machines have none — so it yields
// nothing.
func DiscoverTranscripts(cfg config.Config) []TranscriptFile {
	root := filepath.Join(cfg.ClaudeDir, "projects")
	var files []TranscriptFile

	for _, dir := range readNames(root, true) {
		dirPath := filepath.Join(root, dir)
		for _, name := range readNames(dirPath, false) {
			if !strings.HasSuffix(name, ".jsonl") {
				continue
			}

			filePath := filepath.Join(dirPath, name)
			info, err := os.Stat(filePath)
			if err != nil {
				continue
			}

			files = append(files, TranscriptFile{
				Path:    filePath,
				Dir:     dir,
				Id:      strings.TrimSuffix(name, ".jsonl"),
				MtimeNs: info.ModTime().UnixNano(),
				Size:    info.Size(),
			})
		}
	}

	return files
}

// Sync rebuilds cache/sessions.json, re-reading only the transcripts whose mtime or size moved since
// the last sync. Sessions whose transcript has disappeared are dropped.
func Sync(cfg config.Config, opts SyncOptions) (SyncResult, error) {
	read := opts.ReadLines
	if read == nil {
		read = ReadLines
	}
	now := opts.Now
	if now.IsZero() {
		now = time.Now()
	}

	files := DiscoverTranscripts(cfg)
	cached := map[string]CachedSession{}
	if !opts.Full {
		cached = cachedByPath(cfg)
	}

	sessions := make([]CachedSession, 0, len(files))
	stats := SyncStats{Scanned: len(files)}

	for _, file := range files {
		if previous, found := cached[file.Path]; found && previous.File.MtimeNs == file.MtimeNs && previous.File.Size == file.Size {
			sessions = append(sessions, previous)
			stats.Reused++
			continue
		}

		// A transcript deleted between discovery and read is a race, not a failure: it stays counted
		// as scanned but contributes no record.
		parsed, err := parseTranscript(file, read)
		if err != nil {
			continue
		}

		sessions = append(sessions, parsed)
		stats.Parsed++
	}

	present := make(map[string]struct{}, len(files))
	for _, file := range files {
		present[file.Path] = struct{}{}
	}
	for path := range cached {
		if _, found := present[path]; !found {
			stats.Removed++
		}
	}

	cache := Cache{Version: CacheVersion, SyncedAt: events.FormatTs(now), Sessions: sessions}
	if err := writeCache(cfg, cache); err != nil {
		return SyncResult{}, err
	}

	return SyncResult{Cache: cache, Stats: stats}, nil
}

// Load reads the cache, treating a missing, corrupt or foreign-version file as an empty one.
func Load(cfg config.Config) LoadedCache {
	empty := LoadedCache{Sessions: []CachedSession{}}

	raw, err := os.ReadFile(cfg.SessionsCachePath)
	if err != nil {
		return empty
	}

	// Cache entries are our own writes, but a hand-edited file must not crash a read command, so
	// each record is decoded on its own and a broken one is dropped rather than failing the load.
	var envelope struct {
		Version  int               `json:"version"`
		SyncedAt string            `json:"syncedAt"`
		Sessions []json.RawMessage `json:"sessions"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil || envelope.Version != CacheVersion {
		return empty
	}

	sessions := make([]CachedSession, 0, len(envelope.Sessions))
	for _, record := range envelope.Sessions {
		var session CachedSession
		if err := json.Unmarshal(record, &session); err != nil {
			continue
		}
		if session.Id == "" || session.File.Path == "" {
			continue
		}
		sessions = append(sessions, session)
	}

	return LoadedCache{SyncedAt: envelope.SyncedAt, Sessions: sessions}
}

// CacheAgeMinutes reports the minutes since the last successful sync, or nil when there has never
// been one.
func CacheAgeMinutes(cfg config.Config, now time.Time) *int {
	syncedAt := Load(cfg).SyncedAt
	if syncedAt == "" {
		return nil
	}

	at, err := time.Parse(time.RFC3339, syncedAt)
	if err != nil {
		return nil
	}

	minutes := int(now.Sub(at) / time.Minute)
	return ptrTo(max(0, minutes))
}

func parseTranscript(file TranscriptFile, read LineReader) (CachedSession, error) {
	acc := NewAccumulator()
	for line, err := range read(file.Path) {
		if err != nil {
			return CachedSession{}, err
		}
		acc.FeedLine(line)
	}

	session := acc.Finalize(ptrTo(DecodeProjectSlug(file.Dir)))
	if session.Id == "" {
		// An empty transcript carries no sessionId; the filename is the only id it has.
		session.Id = file.Id
	}

	return CachedSession{
		Session: session,
		File:    FileStat{Path: file.Path, MtimeNs: file.MtimeNs, Size: file.Size},
	}, nil
}

func cachedByPath(cfg config.Config) map[string]CachedSession {
	loaded := Load(cfg)
	index := make(map[string]CachedSession, len(loaded.Sessions))
	for _, session := range loaded.Sessions {
		index[session.File.Path] = session
	}
	return index
}

// writeCache renders the cache the way the rest of waid writes JSON: two-space indent, `<`, `>` and
// `&` left raw, one trailing newline.
func writeCache(cfg config.Config, cache Cache) error {
	if err := os.MkdirAll(cfg.CacheDir, 0o755); err != nil {
		return fmt.Errorf("creating %s: %w", cfg.CacheDir, err)
	}

	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(cache); err != nil {
		return fmt.Errorf("encoding %s: %w", cfg.SessionsCachePath, err)
	}

	if err := os.WriteFile(cfg.SessionsCachePath, buf.Bytes(), 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", cfg.SessionsCachePath, err)
	}
	return nil
}

// readNames lists a directory's entries of one kind, sorted, reporting an unreadable directory as
// an empty one.
func readNames(dirPath string, wantDirs bool) []string {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return nil
	}

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() == wantDirs {
			names = append(names, entry.Name())
		}
	}
	slices.Sort(names)
	return names
}
