package storage

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"unicode/utf8"
)

type Settings struct {
	Mode     int     `json:"mode"`
	FontSize float32 `json:"fontSize"`
	Sidebar  bool    `json:"sidebar"`
	ActiveID string  `json:"activeId"`
}

type Session struct {
	Documents []Document `json:"documents"`
	Recent    []File     `json:"recent"`
	Folder    string     `json:"folder"`
	Settings  Settings   `json:"settings"`
}

type Store struct {
	Directory string
	mu        sync.Mutex
}

func (s *Store) Load() (Session, error) {
	result := Session{Settings: Settings{Mode: 0, FontSize: 15, Sidebar: true}}
	b, err := os.ReadFile(filepath.Join(s.Directory, "session.json"))
	if errors.Is(err, os.ErrNotExist) {
		return result, nil
	}
	if err != nil {
		return result, err
	}
	if err := json.Unmarshal(b, &result); err != nil {
		return result, err
	}
	valid := make([]Document, 0)
	for _, doc := range result.Documents {
		if len(valid) >= 30 {
			break
		}
		if doc.ID == "" || len(doc.Content) > MaxBytes || !utf8.ValidString(doc.Content) {
			continue
		}
		if doc.Path != "" && !doc.Dirty() {
			current, err := Read(doc.Path)
			if err != nil {
				continue
			}
			current.ID = doc.ID
			doc = current
		}
		valid = append(valid, doc)
	}
	result.Documents = valid
	if len(result.Recent) > 15 {
		result.Recent = result.Recent[:15]
	}
	if result.Settings.Mode < 0 || result.Settings.Mode > 2 {
		result.Settings.Mode = 0
	}
	if result.Settings.FontSize < 12 || result.Settings.FontSize > 22 {
		result.Settings.FontSize = 15
	}
	return result, nil
}

func (s *Store) Save(session Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(s.Directory, 0700); err != nil {
		return err
	}
	b, err := json.Marshal(session)
	if err != nil {
		return err
	}
	return AtomicWrite(filepath.Join(s.Directory, "session.json"), b)
}
