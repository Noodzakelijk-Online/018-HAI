package source

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"automation-hub-backend/internal/googleoauth"
	"automation-hub-backend/internal/models"
)

const driveCursorPrefix = "drive:v1:"

type driveCursor struct {
	Version     int    `json:"v"`
	Phase       string `json:"phase"`
	PageToken   string `json:"pageToken,omitempty"`
	ChangeToken string `json:"changeToken,omitempty"`
}

func encodeDriveCursor(cursor driveCursor) (string, error) {
	cursor.Version = 1
	payload, err := json.Marshal(cursor)
	if err != nil {
		return "", err
	}
	return driveCursorPrefix + base64.RawURLEncoding.EncodeToString(payload), nil
}

func decodeDriveCursor(value string) (driveCursor, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return driveCursor{Version: 1, Phase: "backfill"}, nil
	}
	if !strings.HasPrefix(value, driveCursorPrefix) {
		return driveCursor{}, fmt.Errorf("unsupported Drive cursor; reset or reconnect this source")
	}
	payload, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(value, driveCursorPrefix))
	if err != nil {
		return driveCursor{}, fmt.Errorf("decode Drive cursor: %w", err)
	}
	var cursor driveCursor
	if err := json.Unmarshal(payload, &cursor); err != nil {
		return driveCursor{}, fmt.Errorf("decode Drive cursor: %w", err)
	}
	if cursor.Version != 1 || (cursor.Phase != "backfill" && cursor.Phase != "changes") {
		return driveCursor{}, fmt.Errorf("unsupported Drive cursor version or phase")
	}
	return cursor, nil
}

func (s *service) fetchDriveSource(ctx context.Context, source *models.ConnectedSource) ([]ImportItem, string, error) {
	access, err := s.googleAccessToken(ctx, source.ID, driveConnectorKey)
	if err != nil {
		return nil, "", err
	}
	return fetchDriveSourceWithClient(ctx, googleoauth.DriveClient{AccessToken: access}, source)
}

func fetchDriveSourceWithClient(ctx context.Context, client googleoauth.DriveClient, source *models.ConnectedSource) ([]ImportItem, string, error) {
	cursor, err := decodeDriveCursor(source.Cursor)
	if err != nil {
		return nil, "", err
	}
	projectKey := firstNonEmpty(source.DefaultProjectKey, "Robert-life-os")
	if cursor.Phase == "backfill" {
		if cursor.ChangeToken == "" {
			cursor.ChangeToken, err = client.GetStartPageToken(ctx)
			if err != nil {
				return nil, "", fmt.Errorf("capture Drive changes boundary: %w", err)
			}
		}
		page, err := client.ListFilesPage(ctx, cursor.PageToken, driveFetchLimit)
		if err != nil {
			return nil, "", err
		}
		items := driveFilesToImportItems(ctx, client, page.Files, projectKey)
		if page.NextPageToken != "" {
			cursor.PageToken = page.NextPageToken
		} else {
			cursor.Phase = "changes"
			cursor.PageToken = cursor.ChangeToken
			cursor.ChangeToken = ""
		}
		next, err := encodeDriveCursor(cursor)
		return items, next, err
	}

	page, err := client.ListChangesPage(ctx, cursor.PageToken, driveFetchLimit)
	if err != nil {
		return nil, "", err
	}
	items := make([]ImportItem, 0, len(page.Changes))
	for _, change := range page.Changes {
		if change.Removed || change.File == nil || change.File.Trashed {
			items = append(items, ImportItem{
				ExternalID: "drive:" + change.FileID,
				Title:      "Drive item no longer accessible",
				Content:    "Google Drive reports that this item was removed, trashed, or is no longer accessible. Preserve prior conclusions for review; do not treat removal as permission to delete HAI records.",
				SourceURI:  "https://drive.google.com/open?id=" + change.FileID,
				ItemType:   "drive_file_removed",
				ProjectKey: projectKey,
				Metadata:   driveMetadata(change.FileID, "", change.Time, 0, false, true, ""),
			})
			continue
		}
		items = append(items, driveFileToImportItem(ctx, client, *change.File, projectKey))
	}
	nextToken := firstNonEmpty(page.NextPageToken, page.NewStartPageToken)
	if nextToken == "" {
		return nil, "", fmt.Errorf("Drive changes response returned no continuation token")
	}
	cursor.PageToken = nextToken
	next, err := encodeDriveCursor(cursor)
	return items, next, err
}

func driveFilesToImportItems(ctx context.Context, client googleoauth.DriveClient, files []googleoauth.DriveFile, projectKey string) []ImportItem {
	items := make([]ImportItem, 0, len(files))
	for _, file := range files {
		if file.Trashed || file.MimeType == "application/vnd.google-apps.folder" {
			continue
		}
		items = append(items, driveFileToImportItem(ctx, client, file, projectKey))
	}
	return items
}

func driveFileToImportItem(ctx context.Context, client googleoauth.DriveClient, file googleoauth.DriveFile, projectKey string) ImportItem {
	text, fetched, fetchErr := client.FetchText(ctx, file)
	content := strings.Join([]string{
		"Google Drive file: " + firstNonEmpty(file.Name, "(untitled)"),
		"MIME type: " + firstNonEmpty(file.MimeType, "unknown"),
		"Modified: " + formatOptionalTime(file.ModifiedTime),
	}, "\n")
	if fetched && strings.TrimSpace(text) != "" {
		content += "\n\nExtracted content:\n" + text
	}
	fetchError := ""
	if fetchErr != nil {
		fetchError = compact(fetchErr.Error(), 180)
		content += "\n\nContent extraction is pending review: " + fetchError
	}
	sourceURI := firstNonEmpty(file.WebViewLink, "https://drive.google.com/open?id="+file.ID)
	return ImportItem{
		ExternalID: "drive:" + file.ID,
		Title:      firstNonEmpty(file.Name, "(untitled Drive file)"),
		Content:    content,
		SourceURI:  sourceURI,
		ItemType:   "drive_file",
		ProjectKey: projectKey,
		Metadata:   driveMetadata(file.ID, file.MimeType, file.ModifiedTime, file.Size, fetched, false, fetchError),
	}
}

func driveMetadata(fileID, mimeType string, modified time.Time, size int64, contentFetched, removed bool, fetchError string) string {
	metadata := map[string]any{
		"source": "google-drive", "fileId": fileID, "mimeType": mimeType,
		"modifiedTime": formatOptionalTime(modified), "size": size,
		"contentFetched": contentFetched, "removed": removed, "readonly": true,
	}
	if fetchError != "" {
		metadata["contentFetchError"] = fetchError
	}
	payload, _ := json.Marshal(metadata)
	return string(payload)
}

func formatOptionalTime(value time.Time) string {
	if value.IsZero() {
		return "unknown"
	}
	return value.UTC().Format(time.RFC3339)
}
