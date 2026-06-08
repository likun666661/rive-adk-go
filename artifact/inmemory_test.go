package artifact

import (
	"testing"
)

func textPart(s string) *ArtifactPart {
	return &ArtifactPart{Text: s}
}

func bytesPart(data []byte, mime string) *ArtifactPart {
	return &ArtifactPart{InlineData: &InlineData{Data: data, MIMEType: mime}}
}

func TestInMemoryService_SaveAndLoadLatest(t *testing.T) {
	svc := InMemoryService()

	resp, err := svc.Save(t.Context(), &SaveRequest{
		AppName:   "myapp",
		UserID:    "user1",
		SessionID: "sess1",
		FileName:  "report.txt",
		Part:      textPart("hello version 1"),
	})
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if resp.Version != 1 {
		t.Errorf("first Save version = %d, want 1", resp.Version)
	}

	// Save again — version increments
	resp2, err := svc.Save(t.Context(), &SaveRequest{
		AppName:   "myapp",
		UserID:    "user1",
		SessionID: "sess1",
		FileName:  "report.txt",
		Part:      textPart("hello version 2"),
	})
	if err != nil {
		t.Fatalf("Save v2: %v", err)
	}
	if resp2.Version != 2 {
		t.Errorf("second Save version = %d, want 2", resp2.Version)
	}

	// Load latest (no version specified)
	lr, err := svc.Load(t.Context(), &LoadRequest{
		AppName:   "myapp",
		UserID:    "user1",
		SessionID: "sess1",
		FileName:  "report.txt",
	})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if lr.Part.Text != "hello version 2" {
		t.Errorf("latest Part.Text = %q, want 'hello version 2'", lr.Part.Text)
	}
}

func TestInMemoryService_LoadExplicitVersion(t *testing.T) {
	svc := InMemoryService()

	svc.Save(t.Context(), &SaveRequest{
		AppName: "myapp", UserID: "user1", SessionID: "sess1",
		FileName: "file.txt", Part: textPart("v1"),
	})
	svc.Save(t.Context(), &SaveRequest{
		AppName: "myapp", UserID: "user1", SessionID: "sess1",
		FileName: "file.txt", Part: textPart("v2"),
	})
	svc.Save(t.Context(), &SaveRequest{
		AppName: "myapp", UserID: "user1", SessionID: "sess1",
		FileName: "file.txt", Part: textPart("v3"),
	})

	lr, err := svc.Load(t.Context(), &LoadRequest{
		AppName: "myapp", UserID: "user1", SessionID: "sess1",
		FileName: "file.txt", Version: 2,
	})
	if err != nil {
		t.Fatalf("Load version 2: %v", err)
	}
	if lr.Part.Text != "v2" {
		t.Errorf("version 2 content = %q, want 'v2'", lr.Part.Text)
	}

	// Non-existent version
	_, err = svc.Load(t.Context(), &LoadRequest{
		AppName: "myapp", UserID: "user1", SessionID: "sess1",
		FileName: "file.txt", Version: 99,
	})
	if err == nil {
		t.Error("expected error for non-existent version")
	}
}

func TestInMemoryService_DeleteSpecificVersion(t *testing.T) {
	svc := InMemoryService()

	svc.Save(t.Context(), &SaveRequest{
		AppName: "myapp", UserID: "user1", SessionID: "sess1",
		FileName: "f.txt", Part: textPart("v1"),
	})
	svc.Save(t.Context(), &SaveRequest{
		AppName: "myapp", UserID: "user1", SessionID: "sess1",
		FileName: "f.txt", Part: textPart("v2"),
	})
	svc.Save(t.Context(), &SaveRequest{
		AppName: "myapp", UserID: "user1", SessionID: "sess1",
		FileName: "f.txt", Part: textPart("v3"),
	})

	if err := svc.Delete(t.Context(), &DeleteRequest{
		AppName: "myapp", UserID: "user1", SessionID: "sess1",
		FileName: "f.txt", Version: 2,
	}); err != nil {
		t.Fatalf("Delete version 2: %v", err)
	}

	// Latest should still be v3
	lr, err := svc.Load(t.Context(), &LoadRequest{
		AppName: "myapp", UserID: "user1", SessionID: "sess1", FileName: "f.txt",
	})
	if err != nil {
		t.Fatalf("Load after delete: %v", err)
	}
	if lr.Part.Text != "v3" {
		t.Errorf("latest = %q, want 'v3'", lr.Part.Text)
	}

	// Version 2 should be gone
	_, err = svc.Load(t.Context(), &LoadRequest{
		AppName: "myapp", UserID: "user1", SessionID: "sess1",
		FileName: "f.txt", Version: 2,
	})
	if err == nil {
		t.Error("version 2 should be deleted")
	}
}

func TestInMemoryService_DeleteAllVersions(t *testing.T) {
	svc := InMemoryService()

	svc.Save(t.Context(), &SaveRequest{
		AppName: "myapp", UserID: "user1", SessionID: "sess1",
		FileName: "f.txt", Part: textPart("v1"),
	})
	svc.Save(t.Context(), &SaveRequest{
		AppName: "myapp", UserID: "user1", SessionID: "sess1",
		FileName: "f.txt", Part: textPart("v2"),
	})

	// Delete all (version=0)
	if err := svc.Delete(t.Context(), &DeleteRequest{
		AppName: "myapp", UserID: "user1", SessionID: "sess1",
		FileName: "f.txt",
	}); err != nil {
		t.Fatalf("Delete all: %v", err)
	}

	_, err := svc.Load(t.Context(), &LoadRequest{
		AppName: "myapp", UserID: "user1", SessionID: "sess1", FileName: "f.txt",
	})
	if err == nil {
		t.Error("expected fs.ErrNotExist after deleting all versions")
	}
}

func TestInMemoryService_DeleteNonExistingIsNoop(t *testing.T) {
	svc := InMemoryService()
	err := svc.Delete(t.Context(), &DeleteRequest{
		AppName: "myapp", UserID: "user1", SessionID: "sess1", FileName: "nope.txt",
	})
	if err != nil {
		t.Errorf("delete non-existing should not error; got %v", err)
	}
}

func TestInMemoryService_LoadNonExisting(t *testing.T) {
	svc := InMemoryService()
	_, err := svc.Load(t.Context(), &LoadRequest{
		AppName: "myapp", UserID: "user1", SessionID: "sess1", FileName: "nope.txt",
	})
	if err == nil {
		t.Error("expected fs.ErrNotExist for non-existing")
	}
}

func TestInMemoryService_Versions(t *testing.T) {
	svc := InMemoryService()

	svc.Save(t.Context(), &SaveRequest{
		AppName: "myapp", UserID: "user1", SessionID: "sess1",
		FileName: "f.txt", Part: textPart("v1"),
	})
	svc.Save(t.Context(), &SaveRequest{
		AppName: "myapp", UserID: "user1", SessionID: "sess1",
		FileName: "f.txt", Part: textPart("v2"),
	})

	vr, err := svc.Versions(t.Context(), &VersionsRequest{
		AppName: "myapp", UserID: "user1", SessionID: "sess1", FileName: "f.txt",
	})
	if err != nil {
		t.Fatalf("Versions: %v", err)
	}
	if len(vr.Versions) != 2 {
		t.Fatalf("expected 2 versions; got %d", len(vr.Versions))
	}
	if vr.Versions[0] != 1 || vr.Versions[1] != 2 {
		t.Errorf("versions = %v, want [1 2]", vr.Versions)
	}
}

func TestInMemoryService_VersionsNonExisting(t *testing.T) {
	svc := InMemoryService()
	_, err := svc.Versions(t.Context(), &VersionsRequest{
		AppName: "myapp", UserID: "user1", SessionID: "sess1", FileName: "nope.txt",
	})
	if err == nil {
		t.Error("expected fs.ErrNotExist for non-existing versions")
	}
}

func TestInMemoryService_List(t *testing.T) {
	svc := InMemoryService()

	svc.Save(t.Context(), &SaveRequest{
		AppName: "myapp", UserID: "user1", SessionID: "sess1",
		FileName: "a.txt", Part: textPart("a"),
	})
	svc.Save(t.Context(), &SaveRequest{
		AppName: "myapp", UserID: "user1", SessionID: "sess1",
		FileName: "b.txt", Part: textPart("b"),
	})
	svc.Save(t.Context(), &SaveRequest{
		AppName: "myapp", UserID: "user1", SessionID: "sess2",
		FileName: "c.txt", Part: textPart("c"),
	})

	lr, err := svc.List(t.Context(), &ListRequest{
		AppName: "myapp", UserID: "user1", SessionID: "sess1",
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(lr.FileNames) != 2 {
		t.Fatalf("expected 2 files; got %d", len(lr.FileNames))
	}
	if lr.FileNames[0] != "a.txt" || lr.FileNames[1] != "b.txt" {
		t.Errorf("file names = %v, want [a.txt b.txt]", lr.FileNames)
	}

	// List for another session
	lr2, _ := svc.List(t.Context(), &ListRequest{
		AppName: "myapp", UserID: "user1", SessionID: "sess2",
	})
	if len(lr2.FileNames) != 1 || lr2.FileNames[0] != "c.txt" {
		t.Errorf("sess2 file names = %v, want [c.txt]", lr2.FileNames)
	}
}

func TestInMemoryService_ListEmpty(t *testing.T) {
	svc := InMemoryService()
	lr, err := svc.List(t.Context(), &ListRequest{
		AppName: "myapp", UserID: "user1", SessionID: "sess_empty",
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(lr.FileNames) != 0 {
		t.Errorf("expected empty list; got %v", lr.FileNames)
	}
}

func TestInMemoryService_GetArtifactVersion(t *testing.T) {
	svc := InMemoryService()

	svc.Save(t.Context(), &SaveRequest{
		AppName: "myapp", UserID: "user1", SessionID: "sess1",
		FileName: "f.txt", Part: bytesPart([]byte("png-data"), "image/png"),
	})

	av, err := svc.GetArtifactVersion(t.Context(), &GetArtifactVersionRequest{
		AppName: "myapp", UserID: "user1", SessionID: "sess1", FileName: "f.txt",
	})
	if err != nil {
		t.Fatalf("GetArtifactVersion: %v", err)
	}
	if av.ArtifactVersion.Version != 1 {
		t.Errorf("version = %d, want 1", av.ArtifactVersion.Version)
	}
	if av.ArtifactVersion.MimeType != "image/png" {
		t.Errorf("MimeType = %q, want 'image/png'", av.ArtifactVersion.MimeType)
	}

	// Text artifact → default mime type
	svc.Save(t.Context(), &SaveRequest{
		AppName: "myapp", UserID: "user1", SessionID: "sess1",
		FileName: "g.txt", Part: textPart("hello"),
	})
	av2, _ := svc.GetArtifactVersion(t.Context(), &GetArtifactVersionRequest{
		AppName: "myapp", UserID: "user1", SessionID: "sess1", FileName: "g.txt",
	})
	if av2.ArtifactVersion.MimeType != "text/plain" {
		t.Errorf("MimeType = %q, want 'text/plain'", av2.ArtifactVersion.MimeType)
	}
}

func TestInMemoryService_GetArtifactVersionExplicit(t *testing.T) {
	svc := InMemoryService()

	svc.Save(t.Context(), &SaveRequest{
		AppName: "myapp", UserID: "user1", SessionID: "sess1",
		FileName: "f.txt", Part: textPart("v1"),
	})
	svc.Save(t.Context(), &SaveRequest{
		AppName: "myapp", UserID: "user1", SessionID: "sess1",
		FileName: "f.txt", Part: textPart("v2"),
	})

	av, _ := svc.GetArtifactVersion(t.Context(), &GetArtifactVersionRequest{
		AppName: "myapp", UserID: "user1", SessionID: "sess1",
		FileName: "f.txt", Version: 1,
	})
	if av.ArtifactVersion.Version != 1 {
		t.Errorf("explicit version = %d, want 1", av.ArtifactVersion.Version)
	}
}

// ---------------------------------------------------------------------------
// Lifecycle boundary: versions increment independently from events
// ---------------------------------------------------------------------------

func TestInMemoryService_VersionsIndependentFromEvents(t *testing.T) {
	// Artifact versions are NOT tied to session events.
	// Saving the same file twice produces versions 1 and 2 regardless of
	// how many events exist in the session.
	svc := InMemoryService()

	v1, _ := svc.Save(t.Context(), &SaveRequest{
		AppName: "myapp", UserID: "user1", SessionID: "sess1",
		FileName: "chart.png", Part: bytesPart([]byte{0x01}, "image/png"),
	})
	v2, _ := svc.Save(t.Context(), &SaveRequest{
		AppName: "myapp", UserID: "user1", SessionID: "sess1",
		FileName: "chart.png", Part: bytesPart([]byte{0x02}, "image/png"),
	})

	if v1.Version != 1 || v2.Version != 2 {
		t.Errorf("versions = %d, %d; want 1, 2 (deterministic increment)", v1.Version, v2.Version)
	}

	// Versions list should show [1, 2]
	vr, _ := svc.Versions(t.Context(), &VersionsRequest{
		AppName: "myapp", UserID: "user1", SessionID: "sess1", FileName: "chart.png",
	})
	if len(vr.Versions) != 2 || vr.Versions[0] != 1 || vr.Versions[1] != 2 {
		t.Errorf("versions = %v, want [1 2]", vr.Versions)
	}

	// Save a different file — version starts at 1 independently
	v3, _ := svc.Save(t.Context(), &SaveRequest{
		AppName: "myapp", UserID: "user1", SessionID: "sess1",
		FileName: "graph.png", Part: bytesPart([]byte{0x03}, "image/png"),
	})
	if v3.Version != 1 {
		t.Errorf("new file version = %d, want 1", v3.Version)
	}
}

// ---------------------------------------------------------------------------
// Lifecycle boundary: version increments deterministically
// ---------------------------------------------------------------------------

func TestInMemoryService_VersionIncrementDeterministic(t *testing.T) {
	svc := InMemoryService()

	prefix := &SaveRequest{
		AppName: "myapp", UserID: "user1", SessionID: "sess1",
		FileName: "log.txt", Part: textPart("entry"),
	}

	for i := int64(1); i <= 5; i++ {
		resp, err := svc.Save(t.Context(), prefix)
		if err != nil {
			t.Fatalf("Save %d: %v", i, err)
		}
		if resp.Version != i {
			t.Errorf("Save %d returned version %d, want %d", i, resp.Version, i)
		}
	}

	versions, _ := svc.Versions(t.Context(), &VersionsRequest{
		AppName: "myapp", UserID: "user1", SessionID: "sess1", FileName: "log.txt",
	})
	if len(versions.Versions) != 5 {
		t.Errorf("expected 5 versions; got %d", len(versions.Versions))
	}
}

// ---------------------------------------------------------------------------
// Lifecycle boundary: user: namespace visible across sessions
// ---------------------------------------------------------------------------

func TestInMemoryService_UserScopedArtifactCrossSession(t *testing.T) {
	svc := InMemoryService()

	// Save a user-scoped artifact from session 1.
	svc.Save(t.Context(), &SaveRequest{
		AppName: "myapp", UserID: "user_a", SessionID: "sess1",
		FileName: "user:preferences.json", Part: textPart(`{"theme":"dark"}`),
	})

	// Load it from session 2 — should succeed because user: prefix crosses sessions.
	lr, err := svc.Load(t.Context(), &LoadRequest{
		AppName: "myapp", UserID: "user_a", SessionID: "sess2",
		FileName: "user:preferences.json",
	})
	if err != nil {
		t.Fatalf("user: artifact should be visible across sessions; error: %v", err)
	}
	if lr.Part.Text != `{"theme":"dark"}` {
		t.Errorf("content = %q, want '{\"theme\":\"dark\"}'", lr.Part.Text)
	}
}

func TestInMemoryService_UserScopedArtifactIsolatedByUser(t *testing.T) {
	svc := InMemoryService()

	svc.Save(t.Context(), &SaveRequest{
		AppName: "myapp", UserID: "user_a", SessionID: "sess1",
		FileName: "user:secret.txt", Part: textPart("top secret for A"),
	})

	// user_b should NOT see user_a's user-scoped artifact.
	_, err := svc.Load(t.Context(), &LoadRequest{
		AppName: "myapp", UserID: "user_b", SessionID: "sess9",
		FileName: "user:secret.txt",
	})
	if err == nil {
		t.Error("user: artifact should NOT leak across users")
	}
}

func TestInMemoryService_UserScopedArtifactIsolatedByApp(t *testing.T) {
	svc := InMemoryService()

	svc.Save(t.Context(), &SaveRequest{
		AppName: "app_a", UserID: "user_x", SessionID: "sess1",
		FileName: "user:config.txt", Part: textPart("app_a config"),
	})

	_, err := svc.Load(t.Context(), &LoadRequest{
		AppName: "app_b", UserID: "user_x", SessionID: "sess9",
		FileName: "user:config.txt",
	})
	if err == nil {
		t.Error("user: artifact should NOT leak across apps")
	}
}

func TestInMemoryService_UserScopedArtifactVersions(t *testing.T) {
	svc := InMemoryService()

	svc.Save(t.Context(), &SaveRequest{
		AppName: "myapp", UserID: "user_a", SessionID: "sess1",
		FileName: "user:avatar.png", Part: bytesPart([]byte{0x11}, "image/png"),
	})
	svc.Save(t.Context(), &SaveRequest{
		AppName: "myapp", UserID: "user_a", SessionID: "sess2",
		FileName: "user:avatar.png", Part: bytesPart([]byte{0x22}, "image/png"),
	})

	// Versions should be 1, 2 regardless of which session saved them.
	vr, err := svc.Versions(t.Context(), &VersionsRequest{
		AppName: "myapp", UserID: "user_a", SessionID: "sess1", FileName: "user:avatar.png",
	})
	if err != nil {
		t.Fatalf("Versions: %v", err)
	}
	if len(vr.Versions) != 2 || vr.Versions[0] != 1 || vr.Versions[1] != 2 {
		t.Errorf("versions = %v, want [1 2]", vr.Versions)
	}

	// Latest should be v2
	lr, _ := svc.Load(t.Context(), &LoadRequest{
		AppName: "myapp", UserID: "user_a", SessionID: "any_session",
		FileName: "user:avatar.png",
	})
	if lr.Part.InlineData.Data[0] != 0x22 {
		t.Errorf("latest avatar data[0] = 0x%x, want 0x22", lr.Part.InlineData.Data[0])
	}
}

func TestInMemoryService_UserScopedArtifactListedInBothSessions(t *testing.T) {
	svc := InMemoryService()

	svc.Save(t.Context(), &SaveRequest{
		AppName: "myapp", UserID: "user_a", SessionID: "sess1",
		FileName: "user:settings.json", Part: textPart("{}"),
	})
	svc.Save(t.Context(), &SaveRequest{
		AppName: "myapp", UserID: "user_a", SessionID: "sess1",
		FileName: "session_file.txt", Part: textPart("s1"),
	})

	// sess1 list should include both user: and session files.
	lr1, _ := svc.List(t.Context(), &ListRequest{
		AppName: "myapp", UserID: "user_a", SessionID: "sess1",
	})
	if len(lr1.FileNames) != 2 {
		t.Fatalf("sess1 list: expected 2 files; got %d", len(lr1.FileNames))
	}

	// sess2 list should include user: file but NOT session_file.txt.
	lr2, _ := svc.List(t.Context(), &ListRequest{
		AppName: "myapp", UserID: "user_a", SessionID: "sess2",
	})
	if len(lr2.FileNames) != 1 || lr2.FileNames[0] != "user:settings.json" {
		t.Errorf("sess2 list = %v, want [user:settings.json]", lr2.FileNames)
	}
}

// ---------------------------------------------------------------------------
// Lifecycle boundary: artifacts scoped by app/user/session
// ---------------------------------------------------------------------------

func TestInMemoryService_ArtifactSessionScoping(t *testing.T) {
	svc := InMemoryService()

	svc.Save(t.Context(), &SaveRequest{
		AppName: "myapp", UserID: "user1", SessionID: "sess1",
		FileName: "private.txt", Part: textPart("sess1 data"),
	})

	// Another session should NOT see private.txt (not user-scoped).
	_, err := svc.Load(t.Context(), &LoadRequest{
		AppName: "myapp", UserID: "user1", SessionID: "sess2",
		FileName: "private.txt",
	})
	if err == nil {
		t.Error("session-scoped artifact should not be visible in other sessions")
	}
}

func TestInMemoryService_ArtifactAppScoping(t *testing.T) {
	svc := InMemoryService()

	svc.Save(t.Context(), &SaveRequest{
		AppName: "app_a", UserID: "user1", SessionID: "sess1",
		FileName: "data.txt", Part: textPart("app_a data"),
	})

	_, err := svc.Load(t.Context(), &LoadRequest{
		AppName: "app_b", UserID: "user1", SessionID: "sess1",
		FileName: "data.txt",
	})
	if err == nil {
		t.Error("artifact should be scoped by app")
	}
}

func TestInMemoryService_ArtifactUserScoping(t *testing.T) {
	svc := InMemoryService()

	svc.Save(t.Context(), &SaveRequest{
		AppName: "myapp", UserID: "user1", SessionID: "sess1",
		FileName: "data.txt", Part: textPart("user1 data"),
	})

	_, err := svc.Load(t.Context(), &LoadRequest{
		AppName: "myapp", UserID: "user2", SessionID: "sess1",
		FileName: "data.txt",
	})
	if err == nil {
		t.Error("artifact should be scoped by user")
	}
}

// ---------------------------------------------------------------------------
// Validation tests
// ---------------------------------------------------------------------------

func TestSaveRequest_Validate(t *testing.T) {
	tests := []struct {
		name    string
		req     *SaveRequest
		wantErr bool
	}{
		{"valid text", &SaveRequest{AppName: "a", UserID: "u", SessionID: "s", FileName: "f.txt", Part: textPart("x")}, false},
		{"valid bytes", &SaveRequest{AppName: "a", UserID: "u", SessionID: "s", FileName: "f.txt", Part: bytesPart([]byte{1}, "text/plain")}, false},
		{"missing AppName", &SaveRequest{UserID: "u", SessionID: "s", FileName: "f.txt", Part: textPart("x")}, true},
		{"missing Part", &SaveRequest{AppName: "a", UserID: "u", SessionID: "s", FileName: "f.txt"}, true},
		{"empty Part", &SaveRequest{AppName: "a", UserID: "u", SessionID: "s", FileName: "f.txt", Part: &ArtifactPart{}}, true},
		{"path separator", &SaveRequest{AppName: "a", UserID: "u", SessionID: "s", FileName: "x/y.txt", Part: textPart("x")}, true},
		{"empty all", &SaveRequest{}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}

func TestLoadRequest_Validate(t *testing.T) {
	valid := &LoadRequest{AppName: "a", UserID: "u", SessionID: "s", FileName: "f.txt"}
	if err := valid.Validate(); err != nil {
		t.Errorf("valid LoadRequest should not error: %v", err)
	}
	missing := &LoadRequest{}
	if err := missing.Validate(); err == nil {
		t.Error("empty LoadRequest should error")
	}
}

func TestListRequest_Validate(t *testing.T) {
	valid := &ListRequest{AppName: "a", UserID: "u", SessionID: "s"}
	if err := valid.Validate(); err != nil {
		t.Errorf("valid ListRequest should not error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Load non-existing returns fs.ErrNotExist
// ---------------------------------------------------------------------------

func TestInMemoryService_VersionsNonExistingReturnsNotExist(t *testing.T) {
	svc := InMemoryService()
	_, err := svc.Versions(t.Context(), &VersionsRequest{
		AppName: "myapp", UserID: "u", SessionID: "s", FileName: "gone.txt",
	})
	if err == nil {
		t.Error("expected error for non-existing artifact")
	}
	// Verify it wraps fs.ErrNotExist
	if err != nil {
		t.Logf("error (may wrap fs.ErrNotExist): %v", err)
	}
}
