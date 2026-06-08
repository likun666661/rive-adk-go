// Package artifact provides a versioned file store scoped by app/user/session.
//
// Artifacts support independent versioning (each Save increments the version),
// user-scoped names ("user:" prefix — visible across sessions for the same
// user), and listing without exposing blob content.
//
// Lifecycle separation:
//   - Session events represent ordered conversation history.
//   - Memory entries represent searchable long-term knowledge.
//   - Artifact versions represent named files that evolve independently.
//
// The three stores are intentionally isolated to avoid session bloat,
// memory pollution, and version coupling to session lifetime.
package artifact

import (
	"context"
	"fmt"
	"strings"
)

// Service is the artifact storage service.
type Service interface {
	Save(ctx context.Context, req *SaveRequest) (*SaveResponse, error)
	Load(ctx context.Context, req *LoadRequest) (*LoadResponse, error)
	Delete(ctx context.Context, req *DeleteRequest) error
	List(ctx context.Context, req *ListRequest) (*ListResponse, error)
	Versions(ctx context.Context, req *VersionsRequest) (*VersionsResponse, error)
	GetArtifactVersion(ctx context.Context, req *GetArtifactVersionRequest) (*GetArtifactVersionResponse, error)
}

// ArtifactPart holds the content of an artifact.
// Only one of Text or InlineData should be set.
type ArtifactPart struct {
	Text       string
	InlineData *InlineData
}

// InlineData holds binary artifact content.
type InlineData struct {
	Data     []byte
	MIMEType string
}

// SaveRequest is the parameter for [Service.Save].
type SaveRequest struct {
	AppName, UserID, SessionID, FileName string
	Part                                 *ArtifactPart
	Version                              int64
}

func (req *SaveRequest) Validate() error {
	var missing []string
	for _, f := range []struct{ name, value string }{
		{"AppName", req.AppName},
		{"UserID", req.UserID},
		{"SessionID", req.SessionID},
		{"FileName", req.FileName},
	} {
		if f.value == "" {
			missing = append(missing, f.name)
		}
	}
	if req.Part == nil {
		missing = append(missing, "Part")
	}
	if len(missing) > 0 {
		return fmt.Errorf("invalid save request: missing required fields: %s", strings.Join(missing, ", "))
	}
	if req.Part != nil && req.Part.Text == "" && req.Part.InlineData == nil {
		return fmt.Errorf("invalid save request: Part.InlineData or Part.Text must be set")
	}
	if err := validateFileName(req.FileName); err != nil {
		return err
	}
	return nil
}

// SaveResponse is the return type of [Service.Save].
type SaveResponse struct {
	Version int64
}

// LoadRequest is the parameter for [Service.Load].
type LoadRequest struct {
	AppName, UserID, SessionID, FileName string
	Version                              int64
}

func (req *LoadRequest) Validate() error {
	var missing []string
	for _, f := range []struct{ name, value string }{
		{"AppName", req.AppName},
		{"UserID", req.UserID},
		{"SessionID", req.SessionID},
		{"FileName", req.FileName},
	} {
		if f.value == "" {
			missing = append(missing, f.name)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("invalid load request: missing required fields: %s", strings.Join(missing, ", "))
	}
	if err := validateFileName(req.FileName); err != nil {
		return err
	}
	return nil
}

// LoadResponse is the return type of [Service.Load].
type LoadResponse struct {
	Part *ArtifactPart
}

// DeleteRequest is the parameter for [Service.Delete].
type DeleteRequest struct {
	AppName, UserID, SessionID, FileName string
	Version                              int64
}

func (req *DeleteRequest) Validate() error {
	var missing []string
	for _, f := range []struct{ name, value string }{
		{"AppName", req.AppName},
		{"UserID", req.UserID},
		{"SessionID", req.SessionID},
		{"FileName", req.FileName},
	} {
		if f.value == "" {
			missing = append(missing, f.name)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("invalid delete request: missing required fields: %s", strings.Join(missing, ", "))
	}
	if err := validateFileName(req.FileName); err != nil {
		return err
	}
	return nil
}

// ListRequest is the parameter for [Service.List].
type ListRequest struct {
	AppName, UserID, SessionID string
}

func (req *ListRequest) Validate() error {
	var missing []string
	for _, f := range []struct{ name, value string }{
		{"AppName", req.AppName},
		{"UserID", req.UserID},
		{"SessionID", req.SessionID},
	} {
		if f.value == "" {
			missing = append(missing, f.name)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("invalid list request: missing required fields: %s", strings.Join(missing, ", "))
	}
	return nil
}

// ListResponse is the return type of [Service.List].
type ListResponse struct {
	FileNames []string
}

// VersionsRequest is the parameter for [Service.Versions].
type VersionsRequest struct {
	AppName, UserID, SessionID, FileName string
}

func (req *VersionsRequest) Validate() error {
	var missing []string
	for _, f := range []struct{ name, value string }{
		{"AppName", req.AppName},
		{"UserID", req.UserID},
		{"SessionID", req.SessionID},
		{"FileName", req.FileName},
	} {
		if f.value == "" {
			missing = append(missing, f.name)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("invalid versions request: missing required fields: %s", strings.Join(missing, ", "))
	}
	if err := validateFileName(req.FileName); err != nil {
		return err
	}
	return nil
}

// VersionsResponse is the return type of [Service.Versions].
type VersionsResponse struct {
	Versions []int64
}

// ArtifactVersion describes a specific version of an artifact.
type ArtifactVersion struct {
	Version        int64
	CanonicalURI   string
	CustomMetadata map[string]any
	CreateTime     float64
	MimeType       string
}

// GetArtifactVersionRequest is the parameter for [Service.GetArtifactVersion].
type GetArtifactVersionRequest struct {
	AppName, UserID, SessionID, FileName string
	Version                              int64
}

func (req *GetArtifactVersionRequest) Validate() error {
	var missing []string
	for _, f := range []struct{ name, value string }{
		{"AppName", req.AppName},
		{"UserID", req.UserID},
		{"SessionID", req.SessionID},
		{"FileName", req.FileName},
	} {
		if f.value == "" {
			missing = append(missing, f.name)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("invalid get artifact version request: missing required fields: %s", strings.Join(missing, ", "))
	}
	if err := validateFileName(req.FileName); err != nil {
		return err
	}
	return nil
}

// GetArtifactVersionResponse is the return type of [Service.GetArtifactVersion].
type GetArtifactVersionResponse struct {
	ArtifactVersion *ArtifactVersion
}

func validateFileName(name string) error {
	if strings.Contains(name, "/") || strings.Contains(name, "\\") {
		return fmt.Errorf("invalid name: filename cannot contain path separators")
	}
	return nil
}
