package s3

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awscfg "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/dibou/mcp-obsidian/internal/config"
	"github.com/dibou/mcp-obsidian/internal/state"
	syncapi "github.com/dibou/mcp-obsidian/internal/sync"
	"github.com/dibou/mcp-obsidian/internal/vault"
)

type Syncer struct {
	cfg    config.S3Config
	client *s3.Client
	vault  *vault.Service
	store  state.Store
	mu     sync.Mutex
}

func New(ctx context.Context, cfg config.S3Config, v *vault.Service) (*Syncer, error) {
	options := []func(*awscfg.LoadOptions) error{
		awscfg.WithRegion(cfg.Region),
	}
	if cfg.AccessKeyID != "" || cfg.SecretAccessKey != "" {
		options = append(options, awscfg.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.SecretAccessKey, cfg.SessionToken)))
	}
	awsConfig, err := awscfg.LoadDefaultConfig(ctx, options...)
	if err != nil {
		return nil, err
	}
	client := s3.NewFromConfig(awsConfig, func(o *s3.Options) {
		if cfg.Endpoint != "" {
			o.BaseEndpoint = aws.String(cfg.Endpoint)
		}
		o.UsePathStyle = cfg.ForcePathStyle
	})
	return &Syncer{cfg: cfg, client: client, vault: v, store: state.New(v.Root())}, nil
}

func (s *Syncer) Enabled() bool {
	return true
}

func (s *Syncer) EnsureFresh(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	st, err := s.store.Load()
	if err != nil {
		return err
	}
	empty, err := s.vault.IsMarkdownEmpty()
	if err != nil {
		return err
	}
	if empty || s.cfg.SyncInterval == 0 || (!st.LastPull.IsZero() && time.Since(st.LastPull) > s.cfg.SyncInterval) {
		return s.pullLocked(ctx, st)
	}
	return nil
}

func (s *Syncer) Pull(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	st, err := s.store.Load()
	if err != nil {
		return err
	}
	return s.pullLocked(ctx, st)
}

func (s *Syncer) Push(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	st, err := s.store.Load()
	if err != nil {
		return err
	}
	return s.pushLocked(ctx, st)
}

func (s *Syncer) Status(context.Context) (syncapi.Status, error) {
	st, err := s.store.Load()
	if err != nil {
		return syncapi.Status{}, err
	}
	return syncapi.FromState(true, st), nil
}

func (s *Syncer) pullLocked(ctx context.Context, st state.Status) error {
	log.Printf("starting S3 pull from s3://%s/%s", s.cfg.Bucket, s.cfg.Prefix)
	remote, err := s.listRemote(ctx)
	if err != nil {
		s.store.MarkError(err)
		return err
	}
	next := st
	if next.Entries == nil {
		next.Entries = map[string]state.FileEntry{}
	}
	for rel, obj := range remote {
		localPath := filepath.Join(s.vault.Root(), filepath.FromSlash(rel))
		localInfo, statErr := os.Stat(localPath)
		entry, tracked := next.Entries[rel]
		needsDownload := errors.Is(statErr, os.ErrNotExist)
		if statErr == nil {
			if !tracked {
				continue
			}
			localUnchanged := localInfo.Size() == entry.LocalSize && localInfo.ModTime().UTC().Equal(entry.LocalModTime)
			remoteChanged := obj.ETag != entry.ETag || obj.Size != entry.Size || !sameTime(obj.LastModified, entry.LastModified)
			needsDownload = localUnchanged && remoteChanged
		} else if !errors.Is(statErr, os.ErrNotExist) {
			s.store.MarkError(statErr)
			return statErr
		}
		if needsDownload {
			if err := s.download(ctx, rel); err != nil {
				s.store.MarkError(err)
				return err
			}
			localInfo, err = os.Stat(localPath)
			if err != nil {
				s.store.MarkError(err)
				return err
			}
		}
		next.Entries[rel] = state.FileEntry{
			Path:         rel,
			ETag:         obj.ETag,
			Size:         obj.Size,
			LastModified: obj.LastModified,
			LocalSize:    localInfo.Size(),
			LocalModTime: localInfo.ModTime().UTC(),
		}
	}
	for rel := range next.Entries {
		if _, ok := remote[rel]; ok {
			continue
		}
		localPath := filepath.Join(s.vault.Root(), filepath.FromSlash(rel))
		entry := next.Entries[rel]
		info, statErr := os.Stat(localPath)
		if statErr == nil && (info.Size() != entry.LocalSize || !info.ModTime().UTC().Equal(entry.LocalModTime)) {
			continue
		}
		if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
			s.store.MarkError(statErr)
			return statErr
		}
		if err := os.Remove(localPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			s.store.MarkError(err)
			return err
		}
		delete(next.Entries, rel)
	}
	next.LastPull = time.Now().UTC()
	next.LastError = ""
	next.LastErrorTime = time.Time{}
	if err := s.store.Save(next); err != nil {
		return err
	}
	log.Printf("S3 pull complete")
	return nil
}

func (s *Syncer) pushLocked(ctx context.Context, st state.Status) error {
	log.Printf("starting S3 push to s3://%s/%s", s.cfg.Bucket, s.cfg.Prefix)
	local, err := s.localMarkdown()
	if err != nil {
		s.store.MarkError(err)
		return err
	}
	next := st
	if next.Entries == nil {
		next.Entries = map[string]state.FileEntry{}
	}
	for rel, info := range local {
		entry := next.Entries[rel]
		changed := info.Size() != entry.LocalSize || !info.ModTime().UTC().Equal(entry.LocalModTime)
		if changed {
			if err := s.upload(ctx, rel); err != nil {
				s.store.MarkError(err)
				return err
			}
			head, err := s.head(ctx, rel)
			if err != nil {
				s.store.MarkError(err)
				return err
			}
			next.Entries[rel] = state.FileEntry{
				Path:         rel,
				ETag:         head.ETag,
				Size:         head.Size,
				LastModified: head.LastModified,
				LocalSize:    info.Size(),
				LocalModTime: info.ModTime().UTC(),
			}
		}
	}
	if s.cfg.DeleteRemote {
		for rel := range next.Entries {
			if _, ok := local[rel]; ok {
				continue
			}
			if err := s.deleteRemote(ctx, rel); err != nil {
				s.store.MarkError(err)
				return err
			}
			delete(next.Entries, rel)
		}
	}
	next.LastPush = time.Now().UTC()
	next.LastError = ""
	next.LastErrorTime = time.Time{}
	if err := s.store.Save(next); err != nil {
		return err
	}
	log.Printf("S3 push complete")
	return nil
}

func (s *Syncer) listRemote(ctx context.Context) (map[string]state.FileEntry, error) {
	result := map[string]state.FileEntry{}
	paginator := s3.NewListObjectsV2Paginator(s.client, &s3.ListObjectsV2Input{
		Bucket: aws.String(s.cfg.Bucket),
		Prefix: aws.String(s.cfg.Prefix),
	})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, obj := range page.Contents {
			key := aws.ToString(obj.Key)
			rel := s.relFromKey(key)
			if rel == "" || !strings.HasSuffix(strings.ToLower(rel), ".md") {
				continue
			}
			clean, err := vault.ValidateNotePath(rel)
			if err != nil {
				continue
			}
			result[clean] = state.FileEntry{
				Path:         clean,
				ETag:         strings.Trim(aws.ToString(obj.ETag), "\""),
				Size:         aws.ToInt64(obj.Size),
				LastModified: aws.ToTime(obj.LastModified).UTC(),
			}
		}
	}
	return result, nil
}

func (s *Syncer) localMarkdown() (map[string]os.FileInfo, error) {
	notes, err := s.vault.List("", -1)
	if err != nil {
		return nil, err
	}
	result := map[string]os.FileInfo{}
	for _, note := range notes {
		info, err := os.Stat(filepath.Join(s.vault.Root(), filepath.FromSlash(note.Path)))
		if err != nil {
			return nil, err
		}
		result[note.Path] = info
	}
	return result, nil
}

func (s *Syncer) download(ctx context.Context, rel string) error {
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.cfg.Bucket),
		Key:    aws.String(s.key(rel)),
	})
	if err != nil {
		return err
	}
	defer out.Body.Close()
	return s.vault.CopyNote(rel, out.Body)
}

func (s *Syncer) upload(ctx context.Context, rel string) error {
	localPath := filepath.Join(s.vault.Root(), filepath.FromSlash(rel))
	file, err := os.Open(localPath)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.cfg.Bucket),
		Key:         aws.String(s.key(rel)),
		Body:        file,
		ContentType: aws.String("text/markdown; charset=utf-8"),
	})
	return err
}

func (s *Syncer) head(ctx context.Context, rel string) (state.FileEntry, error) {
	out, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.cfg.Bucket),
		Key:    aws.String(s.key(rel)),
	})
	if err != nil {
		return state.FileEntry{}, err
	}
	return state.FileEntry{
		Path:         rel,
		ETag:         strings.Trim(aws.ToString(out.ETag), "\""),
		Size:         aws.ToInt64(out.ContentLength),
		LastModified: aws.ToTime(out.LastModified).UTC(),
	}, nil
}

func (s *Syncer) deleteRemote(ctx context.Context, rel string) error {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.cfg.Bucket),
		Key:    aws.String(s.key(rel)),
	})
	var noSuchKey *types.NoSuchKey
	if errors.As(err, &noSuchKey) {
		return nil
	}
	return err
}

func (s *Syncer) key(rel string) string {
	if s.cfg.Prefix == "" {
		return rel
	}
	return s.cfg.Prefix + rel
}

func (s *Syncer) relFromKey(key string) string {
	if s.cfg.Prefix == "" {
		return key
	}
	return strings.TrimPrefix(key, s.cfg.Prefix)
}

func sameTime(a, b time.Time) bool {
	if a.IsZero() && b.IsZero() {
		return true
	}
	return a.UTC().Equal(b.UTC())
}

func (s *Syncer) String() string {
	return fmt.Sprintf("s3://%s/%s", s.cfg.Bucket, s.cfg.Prefix)
}
