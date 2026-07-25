package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type fileInfo struct {
	modTime time.Time
	size    int64
}

// ResourceWatcher monitors a directory for YAML file modifications with debouncing.
type ResourceWatcher struct {
	dir              string
	pollInterval     time.Duration
	debounceDuration time.Duration
	onChange         func(changedFiles []string)

	mu             sync.Mutex
	knownFiles     map[string]fileInfo
	pendingChanges map[string]struct{}
	debounceTimer  *time.Timer

	stopCh chan struct{}
	wg     sync.WaitGroup
}

// NewResourceWatcher constructs a new watcher for the target directory.
func NewResourceWatcher(dir string, pollInterval, debounceDuration time.Duration, onChange func(changedFiles []string)) *ResourceWatcher {
	if pollInterval <= 0 {
		pollInterval = 200 * time.Millisecond
	}
	if debounceDuration <= 0 {
		debounceDuration = 300 * time.Millisecond
	}

	return &ResourceWatcher{
		dir:              dir,
		pollInterval:     pollInterval,
		debounceDuration: debounceDuration,
		onChange:         onChange,
		knownFiles:       make(map[string]fileInfo),
		pendingChanges:   make(map[string]struct{}),
		stopCh:           make(chan struct{}),
	}
}

// Start initializes the file snapshot and launches the background scan loop.
func (w *ResourceWatcher) Start() error {
	w.mu.Lock()
	// Take initial snapshot without triggering events
	if err := w.scanLocked(true); err != nil {
		w.mu.Unlock()
		return fmt.Errorf("watcher initial scan failed: %w", err)
	}
	w.mu.Unlock()

	w.wg.Add(1)
	go w.run()
	return nil
}

// Stop cleanly terminates the background scanning goroutine and cancels pending timers.
func (w *ResourceWatcher) Stop() {
	close(w.stopCh)
	w.wg.Wait()

	w.mu.Lock()
	if w.debounceTimer != nil {
		w.debounceTimer.Stop()
		w.debounceTimer = nil
	}
	w.mu.Unlock()
}

func (w *ResourceWatcher) run() {
	defer w.wg.Done()
	ticker := time.NewTicker(w.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-w.stopCh:
			return
		case <-ticker.C:
			w.mu.Lock()
			_ = w.scanLocked(false)
			w.mu.Unlock()
		}
	}
}

func (w *ResourceWatcher) scanLocked(isInitial bool) error {
	entries, err := os.ReadDir(w.dir)
	if err != nil {
		return err
	}

	currentFiles := make(map[string]fileInfo)
	var detected []string

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		ext := strings.ToLower(filepath.Ext(name))
		if ext != ".yaml" && ext != ".yml" {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		fi := fileInfo{
			modTime: info.ModTime(),
			size:    info.Size(),
		}
		currentFiles[name] = fi

		if !isInitial {
			oldFi, exists := w.knownFiles[name]
			if !exists || oldFi.modTime != fi.modTime || oldFi.size != fi.size {
				detected = append(detected, name)
			}
		}
	}

	if !isInitial {
		// Check for deleted files
		for oldName := range w.knownFiles {
			if _, exists := currentFiles[oldName]; !exists {
				detected = append(detected, oldName)
			}
		}
	}

	w.knownFiles = currentFiles

	if len(detected) > 0 {
		for _, f := range detected {
			w.pendingChanges[f] = struct{}{}
		}

		if w.debounceTimer != nil {
			w.debounceTimer.Stop()
		}
		w.debounceTimer = time.AfterFunc(w.debounceDuration, w.flushPending)
	}

	return nil
}

func (w *ResourceWatcher) flushPending() {
	w.mu.Lock()
	if len(w.pendingChanges) == 0 {
		w.mu.Unlock()
		return
	}

	changedFiles := make([]string, 0, len(w.pendingChanges))
	for f := range w.pendingChanges {
		changedFiles = append(changedFiles, f)
	}
	w.pendingChanges = make(map[string]struct{})
	w.debounceTimer = nil
	w.mu.Unlock()

	sort.Strings(changedFiles)
	if w.onChange != nil {
		w.onChange(changedFiles)
	}
}
