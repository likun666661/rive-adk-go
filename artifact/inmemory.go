package artifact

import (
	"context"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"sync"
)

const userScopedSessionID = "user"

func fileHasUserNamespace(filename string) bool {
	return strings.HasPrefix(filename, "user:")
}

type artifactIdentity struct {
	appName, userID, sessionID, fileName string
}

type versionedPart struct {
	version int64
	part    *ArtifactPart
}

type inMemoryService struct {
	mu    sync.RWMutex
	store map[artifactIdentity][]versionedPart
}

// InMemoryService returns a new thread-safe in-memory artifact service.
func InMemoryService() Service {
	return &inMemoryService{
		store: make(map[artifactIdentity][]versionedPart),
	}
}

func (s *inMemoryService) resolveIdentity(app, user, session, file string) artifactIdentity {
	if fileHasUserNamespace(file) {
		session = userScopedSessionID
	}
	return artifactIdentity{
		appName:   app,
		userID:    user,
		sessionID: session,
		fileName:  file,
	}
}

func (s *inMemoryService) Save(ctx context.Context, req *SaveRequest) (*SaveResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("request validation failed: %w", err)
	}

	identity := s.resolveIdentity(req.AppName, req.UserID, req.SessionID, req.FileName)

	s.mu.Lock()
	defer s.mu.Unlock()

	versions := s.store[identity]
	nextVersion := int64(1)
	if len(versions) > 0 {
		nextVersion = versions[len(versions)-1].version + 1
	}

	cp := *req.Part
	s.store[identity] = append(versions, versionedPart{version: nextVersion, part: &cp})
	return &SaveResponse{Version: nextVersion}, nil
}

func (s *inMemoryService) Load(ctx context.Context, req *LoadRequest) (*LoadResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("request validation failed: %w", err)
	}

	identity := s.resolveIdentity(req.AppName, req.UserID, req.SessionID, req.FileName)

	s.mu.RLock()
	defer s.mu.RUnlock()

	versions := s.store[identity]
	if len(versions) == 0 {
		return nil, fmt.Errorf("artifact not found: %w", fs.ErrNotExist)
	}

	if req.Version > 0 {
		for _, v := range versions {
			if v.version == req.Version {
				return &LoadResponse{Part: v.part}, nil
			}
		}
		return nil, fmt.Errorf("artifact not found: %w", fs.ErrNotExist)
	}

	latest := versions[len(versions)-1]
	return &LoadResponse{Part: latest.part}, nil
}

func (s *inMemoryService) Delete(ctx context.Context, req *DeleteRequest) error {
	if err := req.Validate(); err != nil {
		return fmt.Errorf("request validation failed: %w", err)
	}

	identity := s.resolveIdentity(req.AppName, req.UserID, req.SessionID, req.FileName)

	s.mu.Lock()
	defer s.mu.Unlock()

	versions := s.store[identity]
	if len(versions) == 0 {
		return nil
	}

	if req.Version != 0 {
		for i, v := range versions {
			if v.version == req.Version {
				s.store[identity] = append(versions[:i], versions[i+1:]...)
				return nil
			}
		}
		return nil
	}

	delete(s.store, identity)
	return nil
}

func (s *inMemoryService) List(ctx context.Context, req *ListRequest) (*ListResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("request validation failed: %w", err)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	files := make(map[string]bool)
	for id := range s.store {
		if id.appName != req.AppName || id.userID != req.UserID {
			continue
		}
		if id.sessionID != req.SessionID && id.sessionID != userScopedSessionID {
			continue
		}
		files[id.fileName] = true
	}

	filenames := make([]string, 0, len(files))
	for f := range files {
		filenames = append(filenames, f)
	}
	sort.Strings(filenames)
	return &ListResponse{FileNames: filenames}, nil
}

func (s *inMemoryService) Versions(ctx context.Context, req *VersionsRequest) (*VersionsResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("request validation failed: %w", err)
	}

	identity := s.resolveIdentity(req.AppName, req.UserID, req.SessionID, req.FileName)

	s.mu.RLock()
	defer s.mu.RUnlock()

	versions := s.store[identity]
	if len(versions) == 0 {
		return nil, fmt.Errorf("artifact not found: %w", fs.ErrNotExist)
	}

	vv := make([]int64, len(versions))
	for i, v := range versions {
		vv[i] = v.version
	}
	return &VersionsResponse{Versions: vv}, nil
}

func (s *inMemoryService) GetArtifactVersion(ctx context.Context, req *GetArtifactVersionRequest) (*GetArtifactVersionResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("request validation failed: %w", err)
	}

	identity := s.resolveIdentity(req.AppName, req.UserID, req.SessionID, req.FileName)

	s.mu.RLock()
	defer s.mu.RUnlock()

	versions := s.store[identity]
	if len(versions) == 0 {
		return nil, fmt.Errorf("artifact not found: %w", fs.ErrNotExist)
	}

	var target versionedPart
	if req.Version > 0 {
		found := false
		for _, v := range versions {
			if v.version == req.Version {
				target = v
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("artifact not found: %w", fs.ErrNotExist)
		}
	} else {
		target = versions[len(versions)-1]
	}

	mimeType := "text/plain"
	if target.part.InlineData != nil {
		mimeType = target.part.InlineData.MIMEType
	}

	return &GetArtifactVersionResponse{
		ArtifactVersion: &ArtifactVersion{
			Version:  target.version,
			MimeType: mimeType,
		},
	}, nil
}

var _ Service = (*inMemoryService)(nil)
