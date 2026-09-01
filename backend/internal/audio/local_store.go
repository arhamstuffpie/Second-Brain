package audio

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/arham/ai-second-brain/internal/service"
)

type LocalStore struct {
	root     string
	maxBytes int64
}

func NewLocalStore(root string, maxBytes int64) (*LocalStore, error) {
	if strings.TrimSpace(root) == "" || maxBytes < 1 {
		return nil, fmt.Errorf("valid local media storage configuration is required")
	}
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve media storage path: %w", err)
	}
	if err := os.MkdirAll(absoluteRoot, 0o750); err != nil {
		return nil, fmt.Errorf("create media storage directory: %w", err)
	}
	return &LocalStore{root: absoluteRoot, maxBytes: maxBytes}, nil
}

func (s *LocalStore) Save(ctx context.Context, filename string, content io.Reader) (service.StoredAudio, error) {
	if content == nil {
		return service.StoredAudio{}, fmt.Errorf("media content is required")
	}
	extension := strings.ToLower(filepath.Ext(filepath.Base(filename)))
	temp, err := os.CreateTemp(s.root, "media-*"+extension)
	if err != nil {
		return service.StoredAudio{}, fmt.Errorf("create media file: %w", err)
	}
	path := temp.Name()
	keep := false
	defer func() {
		_ = temp.Close()
		if !keep {
			_ = os.Remove(path)
		}
	}()

	limited := io.LimitReader(content, s.maxBytes+1)
	written, err := copyWithContext(ctx, temp, limited)
	if err != nil {
		return service.StoredAudio{}, fmt.Errorf("store media: %w", err)
	}
	if written == 0 {
		return service.StoredAudio{}, fmt.Errorf("media file is empty")
	}
	if written > s.maxBytes {
		return service.StoredAudio{}, fmt.Errorf("media file exceeds %d bytes", s.maxBytes)
	}
	if err := temp.Sync(); err != nil {
		return service.StoredAudio{}, fmt.Errorf("sync media file: %w", err)
	}
	keep = true
	if _, err := temp.Seek(0, 0); err != nil {
		return service.StoredObject{}, err
	}
	hash := sha256.New()
	if _, err := io.Copy(hash, temp); err != nil {
		return service.StoredObject{}, err
	}
	key := filepath.Base(path)
	return service.StoredObject{Provider: "local", Key: key, Path: key, SizeBytes: written, SHA256: hex.EncodeToString(hash.Sum(nil))}, nil
}

func (s *LocalStore) objectPath(key string) (string, error) {
	clean := filepath.Clean(key)
	if !filepath.IsAbs(clean) {
		clean = filepath.Join(s.root, clean)
	}
	relative, err := filepath.Rel(s.root, clean)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("media path is outside storage root")
	}
	return clean, nil
}

func (s *LocalStore) Open(_ context.Context, key string) (io.ReadCloser, error) {
	clean, err := s.objectPath(key)
	if err != nil {
		return nil, fmt.Errorf("media path is outside storage root")
	}
	file, err := os.Open(clean)
	if err != nil {
		return nil, fmt.Errorf("open media file: %w", err)
	}
	return file, nil
}

func (s *LocalStore) Delete(_ context.Context, key string) error {
	clean, err := s.objectPath(key)
	if err != nil {
		return fmt.Errorf("media path is outside storage root")
	}
	if err := os.Remove(clean); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete media file: %w", err)
	}
	return nil
}

func (s *LocalStore) Exists(_ context.Context, key string) (bool, error) {
	path, err := s.objectPath(key)
	if err != nil {
		return false, err
	}
	_, err = os.Stat(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	return err == nil, err
}
func (s *LocalStore) Checksum(ctx context.Context, key string) (string, error) {
	f, err := s.Open(ctx, key)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err = io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
func (s *LocalStore) SignedDownloadURL(ctx context.Context, key string, _ time.Duration) (string, error) {
	path, err := s.objectPath(key)
	if err != nil {
		return "", err
	}
	ok, err := s.Exists(ctx, key)
	if err != nil || !ok {
		return "", os.ErrNotExist
	}
	return "file://" + path, nil
}
func (s *LocalStore) RestoreStatus(context.Context, string) (string, error) { return "available", nil }

func copyWithContext(ctx context.Context, destination io.Writer, source io.Reader) (int64, error) {
	buffer := make([]byte, 32*1024)
	var written int64
	for {
		if err := ctx.Err(); err != nil {
			return written, err
		}
		count, readErr := source.Read(buffer)
		if count > 0 {
			n, writeErr := destination.Write(buffer[:count])
			written += int64(n)
			if writeErr != nil {
				return written, writeErr
			}
			if n != count {
				return written, io.ErrShortWrite
			}
		}
		if readErr == io.EOF {
			return written, nil
		}
		if readErr != nil {
			return written, readErr
		}
	}
}

var _ service.AudioStore = (*LocalStore)(nil)
var _ service.VideoStore = (*LocalStore)(nil)
